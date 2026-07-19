// Package monitor 周期性扫描渠道，采集余额 / 倍率并写入快照、变化日志和通知。
package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/fausto2022/relaydeck/backend/channel"
	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/notify"
	"github.com/fausto2022/relaydeck/backend/progress"
	"github.com/fausto2022/relaydeck/backend/storage"
)

const (
	rechargeHistoryDays   = 7
	rechargeRecentWindow  = 6 * time.Hour
	rechargeMinimumSpan   = 2 * time.Hour
	rechargeSafetyFactor  = 1.2
	rechargeMinimumAmount = 10.0
	rechargeRoundingUnit  = 10.0
)

type rechargeSuggestion struct {
	RecommendedAmount     float64
	Forecast24Hours       float64
	HourlyUsage           float64
	EstimatedBalanceHours float64
	Basis                 string
}

// Service 监控扫描服务。
type Service struct {
	channels      *storage.Channels
	announcements *storage.UpstreamAnnouncements
	rates         *storage.Rates
	monitorLogs   *storage.MonitorLogs
	channelSvc    *channel.Service
	dispatcher    *notify.Dispatcher
	log           *slog.Logger
}

func NewService(
	channels *storage.Channels,
	announcements *storage.UpstreamAnnouncements,
	rates *storage.Rates,
	monitorLogs *storage.MonitorLogs,
	channelSvc *channel.Service,
	dispatcher *notify.Dispatcher,
	log *slog.Logger,
) *Service {
	return &Service{
		channels:      channels,
		announcements: announcements,
		rates:         rates,
		monitorLogs:   monitorLogs,
		channelSvc:    channelSvc,
		dispatcher:    dispatcher,
		log:           log,
	}
}

// ScanAllBalances 扫描所有启用监控的渠道余额。单个失败不影响其他。
func (s *Service) ScanAllBalances(ctx context.Context) {
	list, err := s.channels.ListMonitorEnabled()
	if err != nil {
		s.log.Error("list channels", "err", err)
		return
	}
	for i := range list {
		c := list[i]
		if err := s.RefreshBalance(ctx, &c); err != nil {
			s.log.Warn("refresh balance failed", "channel", c.Name, "err", err)
			continue
		}
		if err := s.CheckSubscriptionUsageAlerts(ctx, &c); err != nil {
			s.log.Warn("check subscription usage failed", "channel", c.Name, "err", err)
		}
	}
}

// ScanAllRates 扫描所有启用监控的渠道倍率。
func (s *Service) ScanAllRates(ctx context.Context) {
	list, err := s.channels.ListMonitorEnabled()
	if err != nil {
		s.log.Error("list channels", "err", err)
		return
	}
	for i := range list {
		c := list[i]
		if err := s.RefreshRates(ctx, &c); err != nil {
			s.log.Warn("refresh rates failed", "channel", c.Name, "err", err)
		}
	}
}

// RefreshBalance 单个渠道余额刷新，可被 API 手动触发。
func (s *Service) RefreshBalance(ctx context.Context, c *storage.Channel) error {
	resolved, conn, session, err := s.prepare(ctx, c)
	if err != nil {
		s.notifyError(ctx, c, storage.EventLoginFailed, "登录失败", err)
		return err
	}

	progress.Start(ctx, progress.StageBalance, "拉取余额…")
	started := time.Now()
	res, err := conn.GetBalance(ctx, resolved, session)
	finished := time.Now()
	_ = s.monitorLogs.Append(&storage.MonitorLog{
		ChannelID:    c.ID,
		Job:          storage.MonitorJobBalance,
		Success:      err == nil,
		ErrorMessage: errString(err),
		StartedAt:    started,
		FinishedAt:   finished,
	})
	if err != nil {
		progress.Fail(ctx, progress.StageBalance, err.Error())
		s.notifyError(ctx, c, storage.EventMonitorFailed, "余额采集失败", err)
		return err
	}

	sampledAt := res.SampledAt
	if sampledAt.IsZero() {
		sampledAt = time.Now()
	}
	if err := s.channels.UpdateBalance(c.ID, res.Balance, &sampledAt, ""); err != nil {
		return err
	}
	if err := s.rates.AppendBalance(&storage.BalanceSnapshot{
		ChannelID: c.ID,
		Balance:   res.Balance,
		SampledAt: sampledAt,
	}); err != nil {
		s.log.Warn("append balance snapshot failed", "channel", c.Name, "err", err)
	}
	progress.OK(ctx, progress.StageBalance, fmt.Sprintf("当前余额 %.4f", res.Balance),
		map[string]any{"balance": res.Balance})

	progress.Start(ctx, progress.StageCost, "拉取消费…")
	costRes, err := conn.GetCosts(ctx, resolved, session)
	if err != nil {
		progress.Fail(ctx, progress.StageCost, err.Error())
		s.notifyLowBalance(ctx, c, res.Balance, sampledAt, nil)
		s.notifyError(ctx, c, storage.EventMonitorFailed, "消费采集失败", err)
		return err
	}
	if err := s.channels.UpdateCosts(c.ID, costRes.TodayCost, costRes.TotalCost); err != nil {
		progress.Fail(ctx, progress.StageCost, err.Error())
		s.notifyLowBalance(ctx, c, res.Balance, sampledAt, &costRes.TodayCost)
		return err
	}
	if err := s.rates.AppendCost(&storage.CostSnapshot{
		ChannelID: c.ID,
		TodayCost: costRes.TodayCost,
		SampledAt: sampledAt,
	}); err != nil {
		s.log.Warn("append cost snapshot failed", "channel", c.Name, "err", err)
	}
	progress.OK(ctx, progress.StageCost, fmt.Sprintf("今日 %0.4f / 累计 %0.4f", costRes.TodayCost, costRes.TotalCost),
		map[string]any{"today_cost": costRes.TodayCost, "total_cost": costRes.TotalCost})

	s.notifyLowBalance(ctx, c, res.Balance, sampledAt, &costRes.TodayCost)
	return nil
}

func (s *Service) notifyLowBalance(ctx context.Context, c *storage.Channel, balance float64, sampledAt time.Time, todayCost *float64) {
	if c.BalanceThreshold <= 0 || balance >= c.BalanceThreshold {
		return
	}
	fields := []notify.MarkdownField{
		notify.Detail("渠道", c.Name),
		notify.Detail("当前余额", fmt.Sprintf("%.2f 元", balance)),
		notify.Detail("预警阈值", fmt.Sprintf("%.2f 元", c.BalanceThreshold)),
	}
	if todayCost != nil {
		fields = append(fields, notify.Detail("今日已消费", fmt.Sprintf("%.2f 元", *todayCost)))
	}

	snapshots, err := s.rates.CostHistorySince(c.ID, sampledAt.AddDate(0, 0, -rechargeHistoryDays), 0)
	note := "请及时检查上游余额并安排充值，避免渠道因余额不足中断服务。"
	if err != nil {
		s.log.Warn("load cost history for recharge suggestion failed", "channel", c.Name, "err", err)
	} else {
		suggestion := calculateRechargeSuggestion(snapshots, balance, c.BalanceThreshold, todayCost, sampledAt)
		fields = append(fields,
			notify.Detail("预计未来 24 小时消耗", fmt.Sprintf("%.2f 元", suggestion.Forecast24Hours)),
			notify.Detail("建议充值金额", fmt.Sprintf("%.0f 元", suggestion.RecommendedAmount)),
			notify.Detail("估算依据", suggestion.Basis),
		)
		if suggestion.EstimatedBalanceHours > 0 {
			fields = append(fields, notify.Detail("当前余额预计可用", formatBalanceHours(suggestion.EstimatedBalanceHours)))
		}
		note = "建议金额按未来 24 小时预计消耗增加 20% 余量，并保留预警阈值；最低 10 元，按 10 元向上取整。"
	}
	fields = append(fields, notify.Detail("检测时间", sampledAt.Format("2006-01-02 15:04:05")))
	body := notify.MarkdownDetails("渠道余额已低于预警阈值。", fields...) + notify.MarkdownNote("处理建议", note)
	if err := s.dispatcher.Dispatch(ctx, notify.Message{
		Event:     storage.EventBalanceLow,
		ChannelID: c.ID,
		Subject:   fmt.Sprintf("余额不足 · %s", c.Name),
		Body:      body,
	}); err != nil {
		s.log.Warn("dispatch low balance notification failed", "channel", c.Name, "err", err)
	}
}

func calculateRechargeSuggestion(snapshots []storage.CostSnapshot, balance, threshold float64, todayCost *float64, now time.Time) rechargeSuggestion {
	location := now.Location()
	today := dayKey(now.In(location))
	recentSince := now.Add(-rechargeRecentWindow)
	var recentFirst, recentLast *storage.CostSnapshot
	dailyCosts := make(map[string]float64)
	latestTodayCost := 0.0
	for i := range snapshots {
		snapshot := snapshots[i]
		if snapshot.SampledAt.After(now) {
			continue
		}
		key := dayKey(snapshot.SampledAt.In(location))
		if key != today {
			dailyCosts[key] = snapshot.TodayCost
			continue
		}
		latestTodayCost = snapshot.TodayCost
		if snapshot.SampledAt.Before(recentSince) {
			continue
		}
		if recentFirst == nil {
			item := snapshot
			recentFirst = &item
		}
		item := snapshot
		recentLast = &item
	}
	if todayCost != nil {
		latestTodayCost = *todayCost
	}

	recentHourly := 0.0
	if recentFirst != nil && recentLast != nil {
		span := recentLast.SampledAt.Sub(recentFirst.SampledAt)
		delta := recentLast.TodayCost - recentFirst.TodayCost
		if span >= rechargeMinimumSpan && delta > 0 {
			recentHourly = delta / span.Hours()
		}
	}

	historicalDaily := 0.0
	for _, value := range dailyCosts {
		if value > 0 {
			historicalDaily += value
		}
	}
	if len(dailyCosts) > 0 {
		historicalDaily /= float64(len(dailyCosts))
	}

	todayStart := time.Date(now.In(location).Year(), now.In(location).Month(), now.In(location).Day(), 0, 0, 0, 0, location)
	elapsedHours := now.Sub(todayStart).Hours()
	todayHourly := 0.0
	if elapsedHours >= rechargeMinimumSpan.Hours() && latestTodayCost > 0 {
		todayHourly = latestTodayCost / elapsedHours
	}
	hourlyUsage := math.Max(recentHourly, math.Max(historicalDaily/24, todayHourly))
	forecast24Hours := hourlyUsage * 24
	recommendedAmount := (forecast24Hours * rechargeSafetyFactor) + threshold - balance
	if recommendedAmount < rechargeMinimumAmount {
		recommendedAmount = rechargeMinimumAmount
	}
	recommendedAmount = math.Ceil(recommendedAmount/rechargeRoundingUnit) * rechargeRoundingUnit

	basis := "消费样本不足，按预警阈值和最低充值金额估算"
	parts := make([]string, 0, 3)
	if recentHourly > 0 {
		parts = append(parts, fmt.Sprintf("近 6 小时 %.2f 元/小时", recentHourly))
	}
	if len(dailyCosts) > 0 {
		parts = append(parts, fmt.Sprintf("近 %d 天日均 %.2f 元", len(dailyCosts), historicalDaily))
	}
	if todayHourly > 0 {
		parts = append(parts, fmt.Sprintf("今日平均 %.2f 元/小时", todayHourly))
	}
	if len(parts) > 0 {
		basis = strings.Join(parts, "；") + "，取较高值"
	}
	estimatedBalanceHours := 0.0
	if hourlyUsage > 0 && balance > 0 {
		estimatedBalanceHours = balance / hourlyUsage
	}
	return rechargeSuggestion{
		RecommendedAmount:     recommendedAmount,
		Forecast24Hours:       forecast24Hours,
		HourlyUsage:           hourlyUsage,
		EstimatedBalanceHours: estimatedBalanceHours,
		Basis:                 basis,
	}
}

func formatBalanceHours(hours float64) string {
	if hours < 1 {
		return "不足 1 小时"
	}
	if hours < 24 {
		return fmt.Sprintf("%.1f 小时", hours)
	}
	return fmt.Sprintf("%.1f 天", hours/24)
}

func dayKey(value time.Time) string {
	return value.Format("2006-01-02")
}

// RefreshRates 单个渠道倍率刷新，可被 API 手动触发。
func (s *Service) RefreshRates(ctx context.Context, c *storage.Channel) error {
	resolved, conn, session, err := s.prepare(ctx, c)
	if err != nil {
		s.notifyError(ctx, c, storage.EventLoginFailed, "登录失败", err)
		return err
	}

	progress.Start(ctx, progress.StageRates, "拉取分组倍率…")
	started := time.Now()
	results, err := conn.GetRates(ctx, resolved, session)
	finished := time.Now()
	_ = s.monitorLogs.Append(&storage.MonitorLog{
		ChannelID:    c.ID,
		Job:          storage.MonitorJobRates,
		Success:      err == nil,
		ErrorMessage: errString(err),
		StartedAt:    started,
		FinishedAt:   finished,
	})
	if err != nil {
		progress.Fail(ctx, progress.StageRates, err.Error())
		s.notifyError(ctx, c, storage.EventMonitorFailed, "倍率采集失败", err)
		return err
	}

	now := time.Now()
	existing, err := s.rates.ListByChannel(c.ID)
	if err != nil {
		return err
	}
	isFirstSync := len(existing) == 0
	existingByName := make(map[string]storage.RateSnapshot, len(existing))
	for _, snapshot := range existing {
		existingByName[snapshot.ModelName] = snapshot
	}
	seen := make(map[string]struct{}, len(results))
	changes := make([]notify.RateChange, 0, len(results))
	added := make([]notify.RateChange, 0)
	for _, r := range results {
		seen[r.ModelName] = struct{}{}
		prev, err := s.rates.Upsert(&storage.RateSnapshot{
			ChannelID:       c.ID,
			RemoteGroupID:   r.GroupID,
			ModelName:       r.ModelName,
			Description:     r.Description,
			Ratio:           r.Ratio,
			CompletionRatio: r.CompletionRatio,
			LastSeenAt:      now,
		})
		if err != nil {
			s.log.Warn("rate upsert failed", "channel", c.Name, "model", r.ModelName, "err", err)
			continue
		}
		if prev == nil {
			if !isFirstSync {
				added = append(added, notify.RateChange{
					GroupName: r.ModelName,
					NewRatio:  r.Ratio,
					NewComp:   r.CompletionRatio,
					ChangedAt: now,
				})
			}
			continue
		}
		if prev.Ratio == r.Ratio && prev.CompletionRatio == r.CompletionRatio {
			continue
		}
		oldRatio := prev.Ratio
		oldComp := prev.CompletionRatio
		_ = s.rates.AppendChange(&storage.RateChangeLog{
			ChannelID:          c.ID,
			ModelName:          r.ModelName,
			OldRatio:           &oldRatio,
			NewRatio:           r.Ratio,
			OldCompletionRatio: &oldComp,
			NewCompletionRatio: r.CompletionRatio,
			ChangedAt:          now,
		})
		changes = append(changes, notify.RateChange{
			GroupName: r.ModelName,
			OldRatio:  oldRatio,
			NewRatio:  r.Ratio,
			OldComp:   oldComp,
			NewComp:   r.CompletionRatio,
			ChangedAt: now,
		})
	}
	removed := make([]notify.RateChange, 0)
	for _, snapshot := range existingByName {
		if _, ok := seen[snapshot.ModelName]; ok {
			continue
		}
		if err := s.rates.DeleteSnapshot(c.ID, snapshot.ModelName); err != nil {
			s.log.Warn("rate delete failed", "channel", c.Name, "model", snapshot.ModelName, "err", err)
			continue
		}
		removed = append(removed, notify.RateChange{
			GroupName: snapshot.ModelName,
			OldRatio:  snapshot.Ratio,
			OldComp:   snapshot.CompletionRatio,
			ChangedAt: now,
		})
	}
	// 一次扫描的所有变化打包推送：去抖策略（合并 / 涨跌幅过滤）由 Dispatcher.Policy 决定。
	if len(changes) > 0 {
		_ = s.dispatcher.DispatchRateBatch(ctx, c, changes)
	}
	if len(added)+len(removed) > 0 {
		_ = s.dispatcher.DispatchRateStructureBatch(ctx, c, notify.RateStructureChange{
			Added:   added,
			Removed: removed,
		})
	}
	if err := s.syncAnnouncements(ctx, c, resolved, conn, session); err != nil {
		s.log.Warn("sync announcements failed", "channel", c.Name, "err", err)
	}
	progress.OK(ctx, progress.StageRates, fmt.Sprintf("拉到 %d 个分组", len(results)),
		map[string]any{"count": len(results)})
	return nil
}

func (s *Service) CheckSubscriptionUsageAlerts(ctx context.Context, c *storage.Channel) error {
	if c == nil || !c.MonitorEnabled || !c.SubscriptionEnabled || c.Type != storage.ChannelTypeSub2API {
		return nil
	}
	policy := s.dispatcher.Policy()
	if policy.SubscriptionDailyRemainingThresholdPct <= 0 &&
		policy.SubscriptionWeeklyRemainingThresholdPct <= 0 &&
		policy.SubscriptionMonthlyRemainingThresholdPct <= 0 &&
		policy.SubscriptionExpiryThreshold <= 0 {
		return nil
	}
	info, err := s.channelSvc.GetSubscriptionUsage(ctx, c.ID)
	if err != nil {
		progress.Fail(ctx, progress.StageSubscription, err.Error())
		s.notifyError(ctx, c, storage.EventMonitorFailed, "订阅用量采集失败", err)
		return err
	}
	s.dispatchSubscriptionWindowAlert(ctx, c, storage.EventSubscriptionDailyLow, "每日", policy.SubscriptionDailyRemainingThresholdPct, info.Items, func(item connector.SubscriptionUsage) *connector.SubscriptionUsageWindow {
		return item.Daily
	})
	s.dispatchSubscriptionWindowAlert(ctx, c, storage.EventSubscriptionWeeklyLow, "每周", policy.SubscriptionWeeklyRemainingThresholdPct, info.Items, func(item connector.SubscriptionUsage) *connector.SubscriptionUsageWindow {
		return item.Weekly
	})
	s.dispatchSubscriptionWindowAlert(ctx, c, storage.EventSubscriptionMonthlyLow, "每月", policy.SubscriptionMonthlyRemainingThresholdPct, info.Items, func(item connector.SubscriptionUsage) *connector.SubscriptionUsageWindow {
		return item.Monthly
	})
	s.dispatchSubscriptionExpiryAlert(ctx, c, policy.SubscriptionExpiryThreshold, info.Items)
	progress.OK(ctx, progress.StageSubscription, fmt.Sprintf("检查订阅用量 %d 项", len(info.Items)),
		map[string]any{"count": len(info.Items)})
	return nil
}

func (s *Service) dispatchSubscriptionWindowAlert(ctx context.Context, c *storage.Channel, event storage.NotificationEvent, label string, threshold float64, items []connector.SubscriptionUsage, pick func(connector.SubscriptionUsage) *connector.SubscriptionUsageWindow) {
	if threshold <= 0 {
		return
	}
	lines := make([]string, 0)
	for _, item := range items {
		w := pick(item)
		if w == nil || w.LimitUSD <= 0 || w.RemainingPercent > threshold {
			continue
		}
		reset := "—"
		if w.ResetsAt != nil && !w.ResetsAt.IsZero() {
			reset = w.ResetsAt.Format("01-02 15:04")
		}
		lines = append(lines, fmt.Sprintf("%s：已用 %s / %s，剩余 %s（%s），重置 %s",
			notify.MarkdownCode(subscriptionGroupName(item)),
			notify.MarkdownCode(fmt.Sprintf("$%.4f", w.UsedUSD)),
			notify.MarkdownCode(fmt.Sprintf("$%.4f", w.LimitUSD)),
			notify.MarkdownCode(fmt.Sprintf("$%.4f", w.RemainingUSD)),
			notify.MarkdownCode(fmt.Sprintf("%.1f%%", w.RemainingPercent)),
			notify.MarkdownCode(reset)))
	}
	if len(lines) == 0 {
		return
	}
	body := notify.MarkdownDetails(
		"订阅剩余额度已触发预警。",
		notify.Detail("渠道", c.Name),
		notify.Detail("统计周期", label),
		notify.Detail("预警阈值", fmt.Sprintf("剩余 %.1f%%", threshold)),
		notify.Detail("影响订阅数", len(lines)),
	) + notify.MarkdownSection("额度明细", lines) +
		notify.MarkdownNote("处理建议", "请检查订阅额度和重置时间，必要时提前补充额度或切换渠道。")
	_ = s.dispatcher.Dispatch(ctx, notify.Message{
		Event:     event,
		ChannelID: c.ID,
		Subject:   fmt.Sprintf("订阅%s额度不足 · %s", label, c.Name),
		Body:      body,
	})
}

func (s *Service) dispatchSubscriptionExpiryAlert(ctx context.Context, c *storage.Channel, threshold time.Duration, items []connector.SubscriptionUsage) {
	if threshold <= 0 {
		return
	}
	now := time.Now()
	lines := make([]string, 0)
	for _, item := range items {
		if item.ExpiresAt == nil || item.ExpiresAt.IsZero() {
			continue
		}
		remaining := item.ExpiresAt.Sub(now)
		if remaining > threshold {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s：到期时间 %s，剩余 %s",
			notify.MarkdownCode(subscriptionGroupName(item)),
			notify.MarkdownCode(item.ExpiresAt.Format("2006-01-02 15:04:05")),
			notify.MarkdownCode(formatDurationHours(remaining))))
	}
	if len(lines) == 0 {
		return
	}
	body := notify.MarkdownDetails(
		"订阅有效期已进入预警窗口。",
		notify.Detail("渠道", c.Name),
		notify.Detail("预警阈值", fmt.Sprintf("剩余 %.0f 小时", threshold.Hours())),
		notify.Detail("影响订阅数", len(lines)),
	) + notify.MarkdownSection("到期明细", lines) +
		notify.MarkdownNote("处理建议", "请及时续期，避免订阅到期后影响上游请求。")
	_ = s.dispatcher.Dispatch(ctx, notify.Message{
		Event:     storage.EventSubscriptionExpiring,
		ChannelID: c.ID,
		Subject:   fmt.Sprintf("订阅即将到期 · %s", c.Name),
		Body:      body,
	})
}

func subscriptionGroupName(item connector.SubscriptionUsage) string {
	if strings.TrimSpace(item.GroupName) != "" {
		return strings.TrimSpace(item.GroupName)
	}
	if item.GroupID > 0 {
		return fmt.Sprintf("分组 %d", item.GroupID)
	}
	return fmt.Sprintf("订阅 %d", item.ID)
}

func formatDurationHours(d time.Duration) string {
	if d <= 0 {
		return "已到期"
	}
	hours := d.Hours()
	if hours < 1 {
		return fmt.Sprintf("%.0f 分钟", d.Minutes())
	}
	return fmt.Sprintf("%.1f 小时", hours)
}

func (s *Service) prepare(ctx context.Context, c *storage.Channel) (*connector.Channel, connector.Connector, *connector.AuthSession, error) {
	resolved, err := s.channelSvc.Resolve(ctx, c)
	if err != nil {
		return nil, nil, nil, err
	}
	conn, err := connector.For(resolved.Type)
	if err != nil {
		return nil, nil, nil, err
	}
	s.channelSvc.ApplyHTTPConfig(conn)
	s.channelSvc.ApplyProxy(conn, resolved)
	session, err := s.channelSvc.EnsureSession(ctx, c, resolved, conn)
	if err != nil {
		return nil, nil, nil, err
	}
	return resolved, conn, session, nil
}

func (s *Service) notifyError(ctx context.Context, c *storage.Channel, event storage.NotificationEvent, subject string, err error) {
	_ = s.dispatcher.Dispatch(ctx, notify.Message{
		Event:     event,
		ChannelID: c.ID,
		Subject:   fmt.Sprintf("%s · %s", subject, c.Name),
		Body: notify.MarkdownDetails(
			"渠道监控任务执行失败。",
			notify.Detail("渠道", c.Name),
			notify.Detail("任务", subject),
			notify.Detail("错误", err.Error()),
			notify.Detail("发生时间", time.Now().Format("2006-01-02 15:04:05")),
		) + notify.MarkdownNote("处理建议", "请检查渠道凭据、上游状态和网络连通性。"),
	})
}

func (s *Service) syncAnnouncements(ctx context.Context, c *storage.Channel, resolved *connector.Channel, conn connector.Connector, session *connector.AuthSession) error {
	if s.announcements == nil {
		return nil
	}
	if c.IgnoreAnnouncements {
		return nil
	}
	items, err := conn.GetAnnouncements(ctx, resolved, session)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	records := make([]storage.UpstreamAnnouncement, 0, len(items))
	for _, item := range items {
		records = append(records, storage.UpstreamAnnouncement{
			ChannelID:       c.ID,
			SourceKey:       item.SourceKey,
			Title:           item.Title,
			Content:         item.Content,
			Type:            item.Type,
			Link:            item.Link,
			PublishedAt:     item.PublishedAt,
			SourceUpdatedAt: item.SourceUpdatedAt,
		})
	}
	existingCount, err := s.announcements.CountByChannel(c.ID)
	if err != nil {
		return err
	}
	newRecords, err := s.announcements.Sync(c.ID, records)
	if err != nil {
		return err
	}
	if existingCount == 0 {
		return nil
	}
	for i := range newRecords {
		rec := newRecords[i]
		_ = s.dispatcher.Dispatch(ctx, notify.Message{
			Event:     storage.EventAnnouncement,
			ChannelID: c.ID,
			Subject:   announcementSubject(c, rec),
			Body:      announcementBody(c, rec),
			Extra: map[string]any{
				"announcement_id": rec.ID,
				"source_key":      rec.SourceKey,
				"title":           rec.Title,
				"type":            rec.Type,
				"link":            rec.Link,
			},
		})
	}
	return nil
}

func announcementSubject(c *storage.Channel, a storage.UpstreamAnnouncement) string {
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = strings.TrimSpace(a.Content)
	}
	if title == "" {
		title = "上游公告"
	}
	if len([]rune(title)) > 40 {
		title = string([]rune(title)[:40])
	}
	return fmt.Sprintf("上游公告 · %s · %s", c.Name, title)
}

func announcementBody(c *storage.Channel, a storage.UpstreamAnnouncement) string {
	publishedAt := "—"
	if a.PublishedAt != nil {
		publishedAt = a.PublishedAt.Format("2006-01-02 15:04:05")
	}
	body := notify.MarkdownDetails(
		"收到上游发布的新公告。",
		notify.Detail("渠道", c.Name),
		notify.Detail("公告类型", a.Type),
		notify.Detail("发布时间", publishedAt),
	)
	if content := strings.TrimSpace(a.Content); content != "" {
		body += "\n\n#### 公告内容\n\n" + content
	}
	if link := strings.TrimSpace(a.Link); link != "" {
		body += "\n\n[查看原公告](" + link + ")"
	}
	return body
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
