package mainstation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/rateranking"
	"github.com/fausto2022/relaydeck/backend/storage"
)

const (
	maximumMarginBasisPoints     = int64(9900)
	autoExpansionMaxTestsPerPool = 3
	autoExpansionFailureCooldown = time.Hour
	autoExpansionErrorCooldown   = time.Hour
	autoExpansionRateFreshness   = 15 * time.Minute
)

var autoExpansionProviderPatterns = []struct {
	platform string
	pattern  *regexp.Regexp
}{
	{platform: "anthropic", pattern: regexp.MustCompile(`(?i)anthropic|claude|sonnet|opus|haiku|kiro|cc\s*max|ccmax|aws`)},
	{platform: "gemini", pattern: regexp.MustCompile(`(?i)gemini|google`)},
	{platform: "grok", pattern: regexp.MustCompile(`(?i)grok|xai`)},
	{platform: "image", pattern: regexp.MustCompile(`(?i)生图|绘图|画图|image|dall[ -]?e|midjourney|flux`)},
	{platform: "openai", pattern: regexp.MustCompile(`(?i)openai|gpt|codex|\bplus\b|\bpro\b|\bteam\b|快速稳定|散户|无限制|测试`)},
}

type autoExpansionCandidate struct {
	channel           storage.Channel
	rate              storage.RateSnapshot
	existingMember    *storage.MainAccountPoolMember
	costMicros        int64
	marginBasisPoints int64
}

type autoExpansionCategory struct {
	classifier *rateranking.Classifier
	rule       *rateranking.Rule
}

func validateAutoExpandMarginBasisPoints(value int64) error {
	if value < 0 || value > maximumMarginBasisPoints {
		return errors.New("自动扩池最低利润率必须在 0% 到 99% 之间")
	}
	return nil
}

func validateAutoExpandConditions(enabled bool, minMarginBasisPoints, minRateMultiplierMicros int64) error {
	if minRateMultiplierMicros < 0 {
		return errors.New("自动扩池最低倍率不能小于 0")
	}
	if enabled && minMarginBasisPoints == 0 && minRateMultiplierMicros == 0 {
		return errors.New("开启自动扩池时，最低倍率和最低利润率至少填写一项")
	}
	return nil
}

func autoExpandBlockedKeywords(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r'
	})
	keywords := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		keyword := strings.ToLower(strings.TrimSpace(part))
		if keyword == "" {
			continue
		}
		if _, exists := seen[keyword]; exists {
			continue
		}
		seen[keyword] = struct{}{}
		keywords = append(keywords, keyword)
	}
	return keywords
}

func normalizeAutoExpandBlockedKeywords(value string) string {
	return strings.Join(autoExpandBlockedKeywords(value), "\n")
}

func isAutoExpansionBlocked(groupName string, keywords []string) bool {
	groupName = strings.ToLower(groupName)
	for _, keyword := range keywords {
		if strings.Contains(groupName, keyword) {
			return true
		}
	}
	return false
}

func (s *Service) validateAutoExpansionCategory(ctx context.Context, ruleID *uint, platform string) error {
	_, err := s.autoExpansionCategory(ctx, ruleID, platform)
	return err
}

func (s *Service) autoExpansionCategory(ctx context.Context, ruleID *uint, platform string) (*autoExpansionCategory, error) {
	if ruleID == nil {
		return &autoExpansionCategory{classifier: rateranking.DefaultClassifier()}, nil
	}
	if s.rateRanking == nil {
		return nil, errors.New("倍率排行分类服务尚未初始化")
	}
	config, err := s.rateRanking.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取倍率排行分类失败：%w", err)
	}
	for i := range config.Rules {
		rule := &config.Rules[i]
		if rule.ID != *ruleID {
			continue
		}
		if !rule.Enabled {
			return nil, fmt.Errorf("自动扩池所选分类“%s”已停用，请重新选择", rule.CategoryName)
		}
		if normalizeHealthPlatform(rule.Provider) != platform {
			return nil, fmt.Errorf("自动扩池所选分类“%s”已不属于当前分组类型，请重新选择", rule.CategoryName)
		}
		return &autoExpansionCategory{classifier: rateranking.NewClassifier(config), rule: rule}, nil
	}
	return nil, errors.New("自动扩池所选分类已删除，请重新选择")
}

func (s *Service) RunAutoExpansion(ctx context.Context) {
	if !s.autoExpandMu.TryLock() {
		return
	}
	defer s.autoExpandMu.Unlock()
	config, err := s.store.GetConfig()
	if err != nil || !config.Enabled {
		return
	}
	pools, err := s.store.ListAllPools()
	if err != nil {
		if s.log != nil {
			s.log.Warn("list pools for automatic expansion", "err", err)
		}
		return
	}
	for i := range pools {
		if ctx.Err() != nil {
			return
		}
		pool := &pools[i]
		if !pool.Enabled || !pool.AutoExpandEnabled {
			continue
		}
		runAt := s.now()
		runErr := validateAutoExpandConditions(true, pool.AutoExpandMinMarginBasisPoints, pool.AutoExpandMinRateMicros)
		if runErr == nil {
			runErr = s.expandPoolFromRates(ctx, pool, runAt)
		}
		errText := ""
		if runErr != nil {
			errText = sanitizeText(runErr.Error())
			if s.log != nil {
				s.log.Warn("automatic main station pool expansion failed", "err", runErr, "pool_id", pool.ID)
			}
		}
		if err := s.store.UpdatePoolAutoExpansionStatus(pool.ID, runAt, errText); err != nil && s.log != nil {
			s.log.Warn("update automatic expansion status", "err", err, "pool_id", pool.ID)
		}
	}
}

func (s *Service) expandPoolFromRates(ctx context.Context, pool *storage.MainAccountPool, now time.Time) error {
	groupIDs, err := s.store.ListPoolGroupIDs(pool.ID)
	if err != nil {
		return err
	}
	if len(groupIDs) != 1 {
		return errors.New("自动扩池要求主站分组与账号池保持一对一")
	}
	group, err := s.targetGroups.FindByID(groupIDs[0])
	if err != nil {
		return err
	}
	if group.Missing || !strings.EqualFold(group.Status, "active") {
		return errors.New("主站分组当前不可用，已跳过自动扩池")
	}
	if unsupportedPricing(group) {
		return errors.New("当前主站分组计费方式不支持自动利润判断")
	}
	platform := normalizeHealthPlatform(pool.Platform)
	category, err := s.autoExpansionCategory(ctx, pool.AutoExpandCategoryRuleID, platform)
	if err != nil {
		return err
	}
	modelSelection := effectiveHealthModelSelection(platform, "", false, s.configuredHealthSettings())
	model := modelSelection.Primary
	if model == "" {
		return fmt.Errorf("尚未配置 %s 类型的全局探活模型", platform)
	}
	if _, err := quickTestAPIModeForModel(platform, model); err != nil {
		return errors.New("当前主站分组类型不支持自动扩池测试")
	}
	saleMicros, _, reason := effectiveSaleMultiplier(group, now)
	if reason != "" {
		return errors.New(reason)
	}
	members, err := s.store.ListMembers(pool.ID)
	if err != nil {
		return err
	}
	candidates, err := s.autoExpansionCandidates(pool, group, members, platform, saleMicros, now, category)
	if err != nil {
		return err
	}
	tested := 0
	for i := range candidates {
		if tested >= autoExpansionMaxTestsPerPool || ctx.Err() != nil {
			break
		}
		candidate := &candidates[i]
		tested++
		evidence := autoExpansionEvidence(group, candidate, saleMicros, pool.AutoExpandMinMarginBasisPoints, pool.AutoExpandMinRateMicros, platform, model, category.rule)
		result, testErr := s.quickTestRate(ctx, candidate.channel.ID, candidate.rate.ID, RateQuickTestInput{
			Platform:       platform,
			Model:          model,
			FallbackModels: modelSelection.Fallbacks,
		}, "scheduler")
		if testErr != nil {
			nextAttemptAt := now.Add(autoExpansionErrorCooldown)
			_ = s.saveAutoExpansionAttempt(pool.ID, group.ID, candidate, "error", sanitizeText(testErr.Error()), now, &nextAttemptAt)
			_ = s.appendAudit(&pool.ID, nil, nil, "auto_expand_test", "scheduler", false, nil, nil, evidence, "", sanitizeText(testErr.Error()))
			continue
		}
		if !result.Usable {
			nextAttemptAt := now.Add(autoExpansionFailureCooldown)
			_ = s.saveAutoExpansionAttempt(pool.ID, group.ID, candidate, "failed", result.Message, now, &nextAttemptAt)
			_ = s.appendAudit(&pool.ID, nil, nil, "auto_expand_test", "scheduler", false, nil, result, evidence, result.Message, "")
			continue
		}
		_ = s.saveAutoExpansionAttempt(pool.ID, group.ID, candidate, "usable", result.Message, now, nil)
		_ = s.appendAudit(&pool.ID, nil, nil, "auto_expand_test", "scheduler", true, nil, result, evidence, result.Message, "")
		finalModel := strings.TrimSpace(result.Model)
		if finalModel == "" {
			finalModel = model
		}
		finalMode, modeErr := quickTestAPIModeForModel(platform, finalModel)
		if modeErr != nil {
			nextAttemptAt := now.Add(autoExpansionErrorCooldown)
			_ = s.saveAutoExpansionAttempt(pool.ID, group.ID, candidate, "error", sanitizeText(modeErr.Error()), now, &nextAttemptAt)
			_ = s.appendAudit(&pool.ID, nil, nil, "auto_expand_member_add", "scheduler", false, nil, result, evidence, "", sanitizeText(modeErr.Error()))
			continue
		}
		var member *storage.MainAccountPoolMember
		var createErr error
		if candidate.existingMember != nil {
			member, createErr = s.SyncMember(ctx, pool.ID, candidate.existingMember.ID)
			if createErr == nil && result.FallbackUsed && member != nil && (!strings.EqualFold(member.HealthModel, finalModel) || member.HealthAPIMode != finalMode || !member.HealthModelAutoSelected) {
				before := *member
				if createErr = s.store.UpdateMemberHealth(member.ID, map[string]any{"health_model": finalModel, "health_model_auto_selected": true, "health_api_mode": finalMode}); createErr == nil {
					member.HealthModel = finalModel
					member.HealthModelAutoSelected = true
					member.HealthAPIMode = finalMode
					_ = s.appendAudit(&pool.ID, &member.ID, member.RemoteAccountID, "health_model_fallback", "auto_expand", true,
						&before, member, map[string]any{"primary_model": result.PrimaryModel, "fallback_model": finalModel}, "自动扩池测试切换备用探测模型", "")
				}
			}
		} else {
			enabled := true
			member, createErr = s.CreateMember(ctx, pool.ID, MemberInput{
				AccountName:             candidate.rate.ModelName,
				OwnershipMode:           "managed",
				SourceChannelID:         candidate.channel.ID,
				SourceGroupID:           candidate.rate.RemoteGroupID,
				SourceGroupName:         candidate.rate.ModelName,
				AllowNameConflict:       true,
				Enabled:                 &enabled,
				Preferred:               boolPointer(false),
				Priority:                1,
				Concurrency:             0,
				RateConvertMode:         "raw",
				RateConvertValue:        1,
				CostAdjustment:          1,
				HealthEnabled:           &enabled,
				HealthModel:             finalModel,
				HealthModelAutoSelected: boolPointer(true),
				HealthAPIMode:           finalMode,
			})
		}
		if createErr != nil {
			status := "create_error"
			next := now.Add(autoExpansionErrorCooldown)
			nextAttemptAt := &next
			if member != nil {
				status = "added_error"
			}
			_ = s.saveAutoExpansionAttempt(pool.ID, group.ID, candidate, status, sanitizeText(createErr.Error()), now, nextAttemptAt)
			var memberID *uint
			var remoteAccountID *int64
			if member != nil {
				memberID = &member.ID
				remoteAccountID = member.RemoteAccountID
			}
			_ = s.appendAudit(&pool.ID, memberID, remoteAccountID, "auto_expand_member_add", "scheduler", false, nil, member, evidence, "", sanitizeText(createErr.Error()))
			continue
		}
		_ = s.saveAutoExpansionAttempt(pool.ID, group.ID, candidate, "added", "已自动加入主站分组", now, nil)
		detail := "已通过利润筛选和快速测试，自动加入主站分组"
		if result.FallbackUsed {
			detail = fmt.Sprintf("主模型不可用，已使用备用模型 %s 通过测试并自动加入主站分组", finalModel)
		}
		if result.AttemptCount > 1 {
			if result.FallbackUsed {
				detail = fmt.Sprintf("主模型不可用，已使用备用模型 %s 通过连续 %d 次测试并自动加入主站分组", finalModel, result.AttemptCount)
			} else {
				detail = fmt.Sprintf("已通过利润筛选和连续 %d 次测试，自动加入主站分组", result.AttemptCount)
			}
		}
		_ = s.appendAudit(&pool.ID, &member.ID, member.RemoteAccountID, "auto_expand_member_add", "scheduler", true, nil, member, evidence, detail, "")
		break
	}
	return nil
}

func (s *Service) autoExpansionCandidates(
	pool *storage.MainAccountPool,
	group *storage.UpstreamSyncTargetGroup,
	members []storage.MainAccountPoolMember,
	platform string,
	saleMicros int64,
	now time.Time,
	category *autoExpansionCategory,
) ([]autoExpansionCandidate, error) {
	channels, err := s.channels.ListMonitorEnabled()
	if err != nil {
		return nil, err
	}
	channelIDs := make([]uint, 0, len(channels))
	channelsByID := make(map[uint]storage.Channel, len(channels))
	for i := range channels {
		channelIDs = append(channelIDs, channels[i].ID)
		channelsByID[channels[i].ID] = channels[i]
	}
	rates, err := s.rates.ListByChannels(channelIDs)
	if err != nil {
		return nil, err
	}
	attempts, err := s.store.ListAutoExpansionAttempts(pool.ID)
	if err != nil {
		return nil, err
	}
	attemptsByRateID := make(map[uint]storage.MainStationAutoExpansionAttempt, len(attempts))
	for i := range attempts {
		attemptsByRateID[attempts[i].RateID] = attempts[i]
	}
	candidates := make([]autoExpansionCandidate, 0)
	blockedKeywords := autoExpandBlockedKeywords(pool.AutoExpandBlockedKeywords)
	for j := range rates {
		rate := rates[j]
		channel, ok := channelsByID[rate.ChannelID]
		if !ok {
			continue
		}
		if rate.LastSeenAt.IsZero() || rate.LastSeenAt.Before(now.Add(-autoExpansionRateFreshness)) || classifyAutoExpansionRate(rate) != platform {
			continue
		}
		if isAutoExpansionBlocked(rate.ModelName, blockedKeywords) {
			continue
		}
		if category.rule != nil && category.classifier.ClassifyWithProvider(platform, rate.ModelName, rate.Description).RuleID != category.rule.ID {
			continue
		}
		effectiveRate := connector.ApplyRechargeMultiplier(rate.Ratio, channel.RechargeMultiplier, channel.RechargeMultiplierMode)
		costMicros := scaleFloat(effectiveRate)
		if costMicros <= 0 || costMicros >= saleMicros {
			continue
		}
		if pool.AutoExpandMinRateMicros > 0 && costMicros < pool.AutoExpandMinRateMicros {
			continue
		}
		marginBasisPoints := profitBasisPoints(saleMicros, costMicros)
		if pool.AutoExpandMinMarginBasisPoints > 0 && marginBasisPoints <= pool.AutoExpandMinMarginBasisPoints {
			continue
		}
		attempt, attemptExists := attemptsByRateID[rate.ID]
		matchingMember := autoExpansionMatchingMember(members, channel.ID, &rate)
		if matchingMember != nil && (matchingMember.Status != "error" || !attemptExists || attempt.Status != "added_error") {
			continue
		}
		if attemptExists && attempt.NextAttemptAt != nil && now.Before(*attempt.NextAttemptAt) && attempt.CostMultiplierMicros == costMicros {
			continue
		}
		candidates = append(candidates, autoExpansionCandidate{
			channel: channel, rate: rate, existingMember: matchingMember,
			costMicros: costMicros, marginBasisPoints: marginBasisPoints,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].marginBasisPoints != candidates[j].marginBasisPoints {
			return candidates[i].marginBasisPoints > candidates[j].marginBasisPoints
		}
		if candidates[i].costMicros != candidates[j].costMicros {
			return candidates[i].costMicros < candidates[j].costMicros
		}
		if candidates[i].channel.ID != candidates[j].channel.ID {
			return candidates[i].channel.ID < candidates[j].channel.ID
		}
		return candidates[i].rate.ID < candidates[j].rate.ID
	})
	return candidates, nil
}

func (s *Service) saveAutoExpansionAttempt(
	poolID, targetGroupID uint,
	candidate *autoExpansionCandidate,
	status, message string,
	at time.Time,
	nextAttemptAt *time.Time,
) error {
	return s.store.UpsertAutoExpansionAttempt(&storage.MainStationAutoExpansionAttempt{
		PoolID: poolID, TargetGroupID: targetGroupID, RateID: candidate.rate.ID, ChannelID: candidate.channel.ID,
		Status: status, CostMultiplierMicros: candidate.costMicros, MarginBasisPoints: candidate.marginBasisPoints,
		LastAttemptAt: at, NextAttemptAt: nextAttemptAt, Message: sanitizeText(message),
	})
}

func autoExpansionEvidence(
	group *storage.UpstreamSyncTargetGroup,
	candidate *autoExpansionCandidate,
	saleMicros, marginThreshold, rateThreshold int64,
	platform, model string,
	categoryRule *rateranking.Rule,
) map[string]any {
	evidence := map[string]any{
		"target_group_id":                group.ID,
		"target_group":                   group.Name,
		"channel_id":                     candidate.channel.ID,
		"channel":                        candidate.channel.Name,
		"rate_id":                        candidate.rate.ID,
		"source_group":                   candidate.rate.ModelName,
		"platform":                       platform,
		"model":                          model,
		"sale_multiplier_micros":         saleMicros,
		"cost_multiplier_micros":         candidate.costMicros,
		"margin_basis_points":            candidate.marginBasisPoints,
		"minimum_margin_basis_points":    marginThreshold,
		"minimum_rate_multiplier_micros": rateThreshold,
	}
	if categoryRule != nil {
		evidence["category_rule_id"] = categoryRule.ID
		evidence["category"] = categoryRule.CategoryName
	}
	return evidence
}

func autoExpansionMatchingMember(members []storage.MainAccountPoolMember, channelID uint, rate *storage.RateSnapshot) *storage.MainAccountPoolMember {
	for i := range members {
		member := &members[i]
		if member.SourceChannelID != channelID || member.Status == "orphaned" || member.BindingStatus == "orphaned" {
			continue
		}
		if member.SourceGroupID != nil && rate.RemoteGroupID != nil && *member.SourceGroupID == *rate.RemoteGroupID {
			return member
		}
		if strings.TrimSpace(member.SourceGroupName) != "" && strings.EqualFold(strings.TrimSpace(member.SourceGroupName), strings.TrimSpace(rate.ModelName)) {
			return member
		}
	}
	return nil
}

func classifyAutoExpansionRate(rate storage.RateSnapshot) string {
	if platform := normalizeHealthPlatform(rate.Platform); platform != "" {
		return platform
	}
	text := rate.ModelName + " " + rate.Description
	for i := range autoExpansionProviderPatterns {
		if autoExpansionProviderPatterns[i].pattern.MatchString(text) {
			return autoExpansionProviderPatterns[i].platform
		}
	}
	return "other"
}

func boolPointer(value bool) *bool {
	return &value
}
