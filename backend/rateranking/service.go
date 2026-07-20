package rateranking

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fausto2022/relaydeck/backend/storage"
)

const (
	MatchModeContains = "contains"
	MatchModeWord     = "word"
	GeneralCategory   = "通用"
)

var Providers = []string{"openai", "anthropic", "gemini", "grok", "image", "other"}

var providerPatterns = []struct {
	provider string
	pattern  *regexp.Regexp
}{
	{provider: "anthropic", pattern: regexp.MustCompile(`(?i)anthropic|claude|sonnet|opus|haiku|kiro|cc\s*max|ccmax|aws`)},
	{provider: "gemini", pattern: regexp.MustCompile(`(?i)gemini|google`)},
	{provider: "grok", pattern: regexp.MustCompile(`(?i)grok|xai`)},
	{provider: "image", pattern: regexp.MustCompile(`(?i)生图|绘图|画图|image|dall[ -]?e|midjourney|flux`)},
	{provider: "openai", pattern: regexp.MustCompile(`(?i)openai|gpt|codex|\bplus\b|\bpro\b|\bteam\b|快速稳定|散户|无限制|测试`)},
}

type Store interface {
	List(ctx context.Context) ([]storage.RateRankingProviderSetting, []storage.RateRankingCategoryRule, error)
	Replace(ctx context.Context, providers []storage.RateRankingProviderSetting, rules []storage.RateRankingCategoryRule) error
}

type ProviderSetting struct {
	Provider         string `json:"provider"`
	IncludeUnmatched bool   `json:"include_unmatched"`
}

type Rule struct {
	ID           uint     `json:"id,omitempty"`
	Provider     string   `json:"provider"`
	CategoryName string   `json:"category_name"`
	Keywords     []string `json:"keywords"`
	MatchMode    string   `json:"match_mode"`
	SortOrder    int      `json:"sort_order"`
	Enabled      bool     `json:"enabled"`
}

type Config struct {
	Providers []ProviderSetting `json:"providers"`
	Rules     []Rule            `json:"rules"`
}

type Classification struct {
	Provider      string
	Category      string
	CategoryOrder int
	Visible       bool
}

type Service struct{ store Store }

func New(store Store) *Service {
	return &Service{store: store}
}

func DefaultConfig() Config {
	providers := make([]ProviderSetting, 0, len(Providers))
	for _, provider := range Providers {
		providers = append(providers, ProviderSetting{Provider: provider, IncludeUnmatched: true})
	}
	return Config{Providers: providers, Rules: []Rule{}}
}

func (s *Service) Get(ctx context.Context) (Config, error) {
	providers, storedRules, err := s.store.List(ctx)
	if err != nil {
		return Config{}, err
	}
	config := DefaultConfig()
	providerIndexes := make(map[string]int, len(config.Providers))
	for i := range config.Providers {
		providerIndexes[config.Providers[i].Provider] = i
	}
	for _, item := range providers {
		if index, ok := providerIndexes[item.Provider]; ok {
			config.Providers[index].IncludeUnmatched = item.IncludeUnmatched
		}
	}
	config.Rules = make([]Rule, 0, len(storedRules))
	for _, item := range storedRules {
		keywords, decodeErr := storage.DecodeRateRankingKeywords(item.KeywordsJSON)
		if decodeErr != nil {
			return Config{}, fmt.Errorf("decode rate ranking rule %d: %w", item.ID, decodeErr)
		}
		config.Rules = append(config.Rules, Rule{
			ID: item.ID, Provider: item.Provider, CategoryName: item.CategoryName,
			Keywords: keywords, MatchMode: item.MatchMode, SortOrder: item.SortOrder, Enabled: item.Enabled,
		})
	}
	return config, nil
}

func (s *Service) Save(ctx context.Context, config Config) (Config, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return Config{}, err
	}
	providers := make([]storage.RateRankingProviderSetting, 0, len(normalized.Providers))
	for _, item := range normalized.Providers {
		providers = append(providers, storage.RateRankingProviderSetting{
			Provider: item.Provider, IncludeUnmatched: item.IncludeUnmatched,
		})
	}
	rules := make([]storage.RateRankingCategoryRule, 0, len(normalized.Rules))
	for _, item := range normalized.Rules {
		keywordsJSON, encodeErr := storage.EncodeRateRankingKeywords(item.Keywords)
		if encodeErr != nil {
			return Config{}, encodeErr
		}
		rules = append(rules, storage.RateRankingCategoryRule{
			Provider: item.Provider, CategoryName: item.CategoryName, KeywordsJSON: keywordsJSON,
			MatchMode: item.MatchMode, SortOrder: item.SortOrder, Enabled: item.Enabled,
		})
	}
	if err := s.store.Replace(ctx, providers, rules); err != nil {
		return Config{}, err
	}
	return s.Get(ctx)
}

func (s *Service) Classifier(ctx context.Context) (*Classifier, error) {
	config, err := s.Get(ctx)
	if err != nil {
		return nil, err
	}
	return NewClassifier(config), nil
}

type Classifier struct {
	providers map[string]ProviderSetting
	rules     map[string][]Rule
}

func NewClassifier(config Config) *Classifier {
	classifier := &Classifier{
		providers: make(map[string]ProviderSetting, len(config.Providers)),
		rules:     make(map[string][]Rule),
	}
	for _, item := range config.Providers {
		classifier.providers[item.Provider] = item
	}
	for _, rule := range config.Rules {
		if rule.Enabled {
			classifier.rules[rule.Provider] = append(classifier.rules[rule.Provider], rule)
		}
	}
	for provider := range classifier.rules {
		sort.SliceStable(classifier.rules[provider], func(i, j int) bool {
			return classifier.rules[provider][i].SortOrder < classifier.rules[provider][j].SortOrder
		})
	}
	return classifier
}

func DefaultClassifier() *Classifier {
	return NewClassifier(DefaultConfig())
}

func (c *Classifier) Classify(modelName, description string) Classification {
	provider := classifyProvider(modelName + " " + description)
	for _, rule := range c.rules[provider] {
		if ruleMatches(rule, modelName) {
			return Classification{
				Provider: provider, Category: rule.CategoryName, CategoryOrder: rule.SortOrder, Visible: true,
			}
		}
	}
	setting, ok := c.providers[provider]
	includeUnmatched := !ok || setting.IncludeUnmatched
	return Classification{Provider: provider, Category: GeneralCategory, CategoryOrder: 1_000_000, Visible: includeUnmatched}
}

func normalizeConfig(config Config) (Config, error) {
	validProviders := make(map[string]struct{}, len(Providers))
	for _, provider := range Providers {
		validProviders[provider] = struct{}{}
	}
	settings := make(map[string]bool, len(Providers))
	for _, item := range DefaultConfig().Providers {
		settings[item.Provider] = item.IncludeUnmatched
	}
	for _, item := range config.Providers {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		if _, ok := validProviders[provider]; !ok {
			return Config{}, fmt.Errorf("不支持的倍率类型：%s", item.Provider)
		}
		settings[provider] = item.IncludeUnmatched
	}

	normalized := Config{Providers: make([]ProviderSetting, 0, len(Providers)), Rules: make([]Rule, 0, len(config.Rules))}
	for _, provider := range Providers {
		normalized.Providers = append(normalized.Providers, ProviderSetting{Provider: provider, IncludeUnmatched: settings[provider]})
	}
	categoryNames := make(map[string]struct{}, len(config.Rules))
	for index, rule := range config.Rules {
		provider := strings.ToLower(strings.TrimSpace(rule.Provider))
		if _, ok := validProviders[provider]; !ok {
			return Config{}, fmt.Errorf("第 %d 条规则的所属类型无效", index+1)
		}
		name := strings.TrimSpace(rule.CategoryName)
		if name == "" || utf8.RuneCountInString(name) > 32 {
			return Config{}, fmt.Errorf("第 %d 条规则的分类名称必须为 1 到 32 个字符", index+1)
		}
		if strings.EqualFold(name, GeneralCategory) {
			return Config{}, fmt.Errorf("“%s”是系统保留分类名称", GeneralCategory)
		}
		nameKey := provider + "\x00" + strings.ToLower(name)
		if _, exists := categoryNames[nameKey]; exists {
			return Config{}, fmt.Errorf("%s 类型下的分类名称不能重复：%s", provider, name)
		}
		categoryNames[nameKey] = struct{}{}
		mode := strings.ToLower(strings.TrimSpace(rule.MatchMode))
		if mode != MatchModeContains && mode != MatchModeWord {
			return Config{}, fmt.Errorf("第 %d 条规则的匹配方式无效", index+1)
		}
		keywords := normalizeKeywords(rule.Keywords)
		if len(keywords) == 0 {
			return Config{}, fmt.Errorf("分类“%s”至少需要一个关键词", name)
		}
		if len(keywords) > 20 {
			return Config{}, fmt.Errorf("分类“%s”最多支持 20 个关键词", name)
		}
		for _, keyword := range keywords {
			if utf8.RuneCountInString(keyword) > 64 {
				return Config{}, fmt.Errorf("分类“%s”的单个关键词最多 64 个字符", name)
			}
		}
		sortOrder := rule.SortOrder
		if sortOrder <= 0 {
			sortOrder = (index + 1) * 10
		}
		normalized.Rules = append(normalized.Rules, Rule{
			Provider: provider, CategoryName: name, Keywords: keywords,
			MatchMode: mode, SortOrder: sortOrder, Enabled: rule.Enabled,
		})
	}
	sort.SliceStable(normalized.Rules, func(i, j int) bool {
		if normalized.Rules[i].Provider != normalized.Rules[j].Provider {
			return providerIndex(normalized.Rules[i].Provider) < providerIndex(normalized.Rules[j].Provider)
		}
		return normalized.Rules[i].SortOrder < normalized.Rules[j].SortOrder
	})
	return normalized, nil
}

func normalizeKeywords(keywords []string) []string {
	result := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		key := strings.ToLower(keyword)
		if keyword == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, keyword)
	}
	return result
}

func classifyProvider(text string) string {
	for _, item := range providerPatterns {
		if item.pattern.MatchString(text) {
			return item.provider
		}
	}
	return "other"
}

func ruleMatches(rule Rule, modelName string) bool {
	text := strings.ToLower(modelName)
	for _, keyword := range rule.Keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" {
			continue
		}
		if rule.MatchMode == MatchModeWord {
			if containsWholeWord(text, keyword) {
				return true
			}
			continue
		}
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func containsWholeWord(text, keyword string) bool {
	for start := 0; start <= len(text)-len(keyword); {
		relative := strings.Index(text[start:], keyword)
		if relative < 0 {
			return false
		}
		index := start + relative
		beforeOK := index == 0 || !isWordRuneBefore(text, index)
		afterIndex := index + len(keyword)
		afterOK := afterIndex == len(text) || !isWordRuneAt(text, afterIndex)
		if beforeOK && afterOK {
			return true
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		start = index + size
	}
	return false
}

func isWordRuneBefore(text string, index int) bool {
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return isWordRune(r)
}

func isWordRuneAt(text string, index int) bool {
	r, _ := utf8.DecodeRuneInString(text[index:])
	return isWordRune(r)
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func providerIndex(provider string) int {
	for index, item := range Providers {
		if item == provider {
			return index
		}
	}
	return len(Providers)
}
