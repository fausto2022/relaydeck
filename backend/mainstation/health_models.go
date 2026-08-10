package mainstation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/storage"
)

var builtinHealthModels = map[string][]string{
	"openai": {
		"gpt-5.2", "gpt-5.2-2025-12-11", "gpt-5.2-chat-latest", "gpt-5.2-pro", "gpt-5.2-pro-2025-12-11",
		"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini",
		"gpt-5.4-2026-03-05", "gpt-5.3-codex-spark", "codex-auto-review", "gpt-4o-audio-preview",
		"gpt-4o-realtime-preview", "gpt-image-1", "gpt-image-1.5", "gpt-image-2",
	},
	"anthropic": {
		"claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20240620", "claude-3-5-haiku-20241022",
		"claude-3-7-sonnet-20250219", "claude-sonnet-4-20250514", "claude-opus-4-20250514",
		"claude-opus-4-1-20250805", "claude-sonnet-4-5-20250929", "claude-haiku-4-5-20251001",
		"claude-opus-4-5-20251101", "claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8",
		"claude-sonnet-4-6", "claude-sonnet-5", "claude-fable-5",
	},
	"gemini": {
		"gemini-3.1-flash-image", "gemini-2.5-flash-image", "gemini-2.0-flash", "gemini-2.5-flash",
		"gemini-2.5-pro", "gemini-3.5-flash", "gemini-3-flash-preview", "gemini-3-pro-preview",
	},
	"grok": {
		"grok-4.5", "grok-4.3", "grok-build-0.1", "grok-composer-2.5-fast", "grok-4.20-0309-reasoning",
		"grok-4.20-0309-non-reasoning", "grok-4.20-multi-agent-0309", "grok", "grok-latest",
		"grok-4.5-latest", "grok-build", "grok-build-latest", "grok-composer", "composer-2.5",
		"grok-4.20-reasoning", "grok-4.20-non-reasoning", "grok-imagine", "grok-imagine-image-quality",
		"grok-imagine-image", "grok-imagine-edit", "grok-imagine-video", "grok-imagine-video-1.5",
	},
	"image": {
		"gpt-image-1", "gpt-image-1.5", "gpt-image-2", "dall-e-3", "dall-e-2",
		"gemini-3.1-flash-image", "gemini-2.5-flash-image", "grok-imagine-image",
	},
}

func decodeHealthModels(raw string) map[string]string {
	models := make(map[string]string)
	_ = json.Unmarshal([]byte(raw), &models)
	return normalizeHealthModels(models)
}

func encodeHealthModels(models map[string]string) (string, error) {
	normalized := normalizeHealthModels(models)
	for platform, model := range normalized {
		if len(platform) > 64 {
			return "", errors.New("health model platform is too long")
		}
		if len(model) > 256 {
			return "", errors.New("health model name is too long")
		}
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validateHealthModelFallbacks(primary, fallback, secondFallback map[string]string) error {
	normalizedPrimary := normalizeHealthModels(primary)
	normalizedFallback := normalizeHealthModels(fallback)
	for platform, model := range normalizedFallback {
		primaryModel := strings.TrimSpace(normalizedPrimary[platform])
		if primaryModel == "" {
			return fmt.Errorf("%s 备用探测模型必须先配置主探测模型", platform)
		}
		if strings.EqualFold(primaryModel, model) {
			return fmt.Errorf("%s 主探测模型和备用探测模型不能相同", platform)
		}
	}
	for platform, model := range normalizeHealthModels(secondFallback) {
		primaryModel := strings.TrimSpace(normalizedPrimary[platform])
		fallbackModel := strings.TrimSpace(normalizedFallback[platform])
		if primaryModel == "" {
			return fmt.Errorf("%s 第三探测模型必须先配置主探测模型", platform)
		}
		if fallbackModel == "" {
			return fmt.Errorf("%s 第三探测模型必须先配置第二探测模型", platform)
		}
		if strings.EqualFold(primaryModel, model) || strings.EqualFold(fallbackModel, model) {
			return fmt.Errorf("%s 三档探测模型不能重复", platform)
		}
	}
	return nil
}

func normalizeHealthModels(models map[string]string) map[string]string {
	normalized := make(map[string]string)
	for platform, model := range models {
		platform = normalizeHealthPlatform(platform)
		model = strings.TrimSpace(model)
		if platform != "" && model != "" {
			normalized[platform] = model
		}
	}
	return normalized
}

func normalizeHealthPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "claude":
		return "anthropic"
	case "google":
		return "gemini"
	case "xai":
		return "grok"
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func effectiveHealthModel(platform, memberModel string, globalModels map[string]string) string {
	if model := strings.TrimSpace(memberModel); model != "" {
		return model
	}
	return strings.TrimSpace(globalModels[normalizeHealthPlatform(platform)])
}

type healthModelSelection struct {
	Primary   string
	Fallbacks []string
}

// 账号显式指定模型时保持该设置的权威性；自动选中的模型会继续继承其后的备用链。
func effectiveHealthModelSelection(platform, memberModel string, autoSelected bool, settings globalHealthSettings) healthModelSelection {
	platform = normalizeHealthPlatform(platform)
	chain := healthModelChain(
		strings.TrimSpace(settings.Models[platform]),
		strings.TrimSpace(settings.FallbackModels[platform]),
		strings.TrimSpace(settings.SecondFallbackModels[platform]),
	)
	if model := strings.TrimSpace(memberModel); model != "" {
		if autoSelected {
			for index, item := range chain {
				if strings.EqualFold(item, model) {
					return healthModelSelection{Primary: item, Fallbacks: append([]string(nil), chain[index+1:]...)}
				}
			}
		}
		return healthModelSelection{Primary: model}
	}
	if len(chain) == 0 {
		return healthModelSelection{}
	}
	return healthModelSelection{Primary: chain[0], Fallbacks: append([]string(nil), chain[1:]...)}
}

func healthModelChain(models ...string) []string {
	chain := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		duplicate := false
		for _, item := range chain {
			if strings.EqualFold(item, model) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			chain = append(chain, model)
		}
	}
	return chain
}

func (s *Service) configuredHealthModels() map[string]string {
	return s.configuredHealthSettings().Models
}

func (s *Service) ListHealthModelCatalogs(ctx context.Context) ([]HealthModelCatalog, error) {
	settings := s.configuredHealthSettings()
	pools, _, err := s.store.ListPools(1, 1000)
	if err != nil {
		return nil, err
	}
	candidates := make(map[string][]storage.MainAccountPoolMember)
	platforms := map[string]struct{}{
		"openai": {}, "anthropic": {}, "gemini": {}, "grok": {}, "image": {},
	}
	for platform := range settings.Models {
		platforms[platform] = struct{}{}
	}
	for platform := range settings.FallbackModels {
		platforms[platform] = struct{}{}
	}
	for platform := range settings.SecondFallbackModels {
		platforms[platform] = struct{}{}
	}
	snapshots, err := s.store.ListAllAccountSnapshots(false)
	if err != nil {
		return nil, err
	}
	_, adminTarget, adminAPIKey, adminErr := s.loadAdminTarget()
	admin := s.adminFactory()
	for i := range pools {
		platform := normalizeHealthPlatform(pools[i].Platform)
		if platform == "" {
			continue
		}
		platforms[platform] = struct{}{}
		members, listErr := s.store.ListMembers(pools[i].ID)
		if listErr != nil {
			return nil, listErr
		}
		candidates[platform] = append(candidates[platform], members...)
	}
	keys := make([]string, 0, len(platforms))
	for platform := range platforms {
		keys = append(keys, platform)
	}
	sort.Strings(keys)
	catalogs := make([]HealthModelCatalog, 0, len(keys))
	for _, platform := range keys {
		catalog := HealthModelCatalog{Platform: platform, Models: append([]string(nil), builtinHealthModels[platform]...)}
		var discovered []string
		var lastErr error
		seen := make(map[string]struct{})
		for i := range candidates[platform] {
			member := &candidates[platform][i]
			if member.SourceAPIKeyID == nil {
				continue
			}
			candidateKey := memberKey(member)
			if _, ok := seen[candidateKey]; ok {
				continue
			}
			seen[candidateKey] = struct{}{}
			channel, secret, credentialErr := s.healthSourceCredentials(ctx, member)
			if credentialErr != nil {
				lastErr = credentialErr
				continue
			}
			discovered, lastErr = connector.FetchModels(ctx, s.probeHTTPClient(channel), channel.SiteURL, platform, secret)
			if lastErr == nil {
				break
			}
		}
		if len(discovered) == 0 && adminErr == nil {
			attempts := 0
			for i := range snapshots {
				if normalizeHealthPlatform(snapshots[i].Platform) != platform {
					continue
				}
				attempts++
				discovered, lastErr = admin.SyncAccountModelsFromUpstream(ctx, sub2api.AdminTarget{
					BaseURL: adminTarget.BaseURL, APIKey: adminAPIKey,
				}, snapshots[i].RemoteAccountID)
				if lastErr != nil {
					discovered, lastErr = admin.ListAccountModels(ctx, sub2api.AdminTarget{
						BaseURL: adminTarget.BaseURL, APIKey: adminAPIKey,
					}, snapshots[i].RemoteAccountID)
				}
				if lastErr == nil && len(discovered) > 0 {
					break
				}
				if attempts >= 3 {
					break
				}
			}
		}
		catalog.Models = mergeHealthModelLists(catalog.Models, discovered)
		if len(catalog.Models) == 0 {
			if lastErr != nil {
				catalog.Error = sanitizeText(redactSecretError(lastErr, adminAPIKey).Error())
			} else {
				catalog.Error = "该平台没有可用于获取模型的账号 API Key"
			}
		}
		catalogs = append(catalogs, catalog)
	}
	return catalogs, nil
}

func mergeHealthModelLists(lists ...[]string) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, list := range lists {
		for _, value := range list {
			model := strings.TrimSpace(value)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}

func memberKey(member *storage.MainAccountPoolMember) string {
	if member == nil || member.SourceAPIKeyID == nil {
		return ""
	}
	return strings.Join([]string{
		strconv.FormatUint(uint64(member.SourceChannelID), 10),
		strconv.FormatInt(*member.SourceAPIKeyID, 10),
	}, ":")
}
