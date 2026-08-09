package rateranking

import (
	"context"
	"testing"

	"github.com/fausto2022/relaydeck/backend/storage"
)

type memoryStore struct {
	providers []storage.RateRankingProviderSetting
	rules     []storage.RateRankingCategoryRule
}

func (s *memoryStore) List(context.Context) ([]storage.RateRankingProviderSetting, []storage.RateRankingCategoryRule, error) {
	return s.providers, s.rules, nil
}

func (s *memoryStore) Replace(_ context.Context, providers []storage.RateRankingProviderSetting, rules []storage.RateRankingCategoryRule) error {
	s.providers = providers
	s.rules = rules
	var nextID uint
	for _, rule := range s.rules {
		if rule.ID > nextID {
			nextID = rule.ID
		}
	}
	for index := range s.rules {
		if s.rules[index].ID == 0 {
			nextID++
			s.rules[index].ID = nextID
		}
	}
	return nil
}

func TestClassifierUsesRulePriorityAndFallback(t *testing.T) {
	config := DefaultConfig()
	for index := range config.Providers {
		if config.Providers[index].Provider == "openai" {
			config.Providers[index].IncludeUnmatched = false
		}
	}
	config.Rules = []Rule{
		{Provider: "openai", CategoryName: "Plus", Keywords: []string{"plus"}, MatchMode: MatchModeWord, SortOrder: 20, Enabled: true},
		{Provider: "openai", CategoryName: "Pro", Keywords: []string{"PRO"}, MatchMode: MatchModeWord, SortOrder: 10, Enabled: true},
	}
	classifier := NewClassifier(config)

	matched := classifier.Classify("OpenAI Pro Plus", "")
	if matched.Provider != "openai" || matched.Category != "Pro" || matched.RuleID != 0 || !matched.Visible {
		t.Fatalf("priority classification = %#v", matched)
	}
	wordMiss := classifier.Classify("OpenAI Profile", "")
	if wordMiss.Category != GeneralCategory || wordMiss.Visible {
		t.Fatalf("whole-word miss = %#v", wordMiss)
	}
	other := classifier.Classify("普通线路", "")
	if other.Provider != "other" || other.Category != GeneralCategory || !other.Visible {
		t.Fatalf("default fallback = %#v", other)
	}
}

func TestServiceSaveKeepsRuleIDWhenRenamed(t *testing.T) {
	store := &memoryStore{}
	service := New(store)
	config := DefaultConfig()
	config.Rules = []Rule{
		{Provider: "openai", CategoryName: "Pro", Keywords: []string{"pro"}, MatchMode: MatchModeContains, Enabled: true},
		{Provider: "openai", CategoryName: "Plus", Keywords: []string{"plus"}, MatchMode: MatchModeContains, Enabled: true},
	}
	saved, err := service.Save(context.Background(), config)
	if err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	plusID := saved.Rules[1].ID
	saved.Rules = []Rule{{
		ID: plusID, Provider: "openai", CategoryName: "Team", Keywords: []string{"team"}, MatchMode: MatchModeContains, Enabled: true,
	}}
	saved, err = service.Save(context.Background(), saved)
	if err != nil {
		t.Fatalf("rename category: %v", err)
	}
	if len(saved.Rules) != 1 || saved.Rules[0].ID != plusID || saved.Rules[0].CategoryName != "Team" {
		t.Fatalf("renamed rules = %#v", saved.Rules)
	}
	classification := NewClassifier(saved).ClassifyWithProvider("openai", "Team 专线", "")
	if classification.RuleID != plusID || classification.Category != "Team" {
		t.Fatalf("classification = %#v", classification)
	}
}

func TestClassifierUsesUpstreamPlatformBeforeNameInference(t *testing.T) {
	config := DefaultConfig()
	config.Rules = []Rule{{
		Provider: "openai", CategoryName: "生图", Keywords: []string{"生图"}, MatchMode: MatchModeContains, SortOrder: 10, Enabled: true,
	}}
	classifier := NewClassifier(config)

	generic := classifier.ClassifyWithProvider("openai", "狂欢", "")
	if generic.Provider != "openai" || generic.Category != GeneralCategory || !generic.Visible {
		t.Fatalf("generic classification = %#v", generic)
	}
	misleading := classifier.ClassifyWithProvider("anthropic", "OpenAI Pro", "")
	if misleading.Provider != "anthropic" {
		t.Fatalf("upstream platform was overridden by name: %#v", misleading)
	}
	image := classifier.ClassifyWithProvider("openai", "高速生图套餐", "")
	if image.Provider != "openai" || image.Category != "生图" {
		t.Fatalf("custom image category = %#v", image)
	}
	xai := classifier.ClassifyWithProvider("xai", "普通套餐", "")
	if xai.Provider != "grok" {
		t.Fatalf("xai alias classification = %#v", xai)
	}
}

func TestServiceSaveNormalizesAndPersistsRules(t *testing.T) {
	store := &memoryStore{}
	service := New(store)
	config := DefaultConfig()
	config.Rules = []Rule{
		{Provider: "openai", CategoryName: " Pro ", Keywords: []string{"PRO", " pro ", ""}, MatchMode: MatchModeContains, Enabled: true},
	}

	saved, err := service.Save(context.Background(), config)
	if err != nil {
		t.Fatalf("save config: %v", err)
	}
	if len(saved.Rules) != 1 || saved.Rules[0].CategoryName != "Pro" || len(saved.Rules[0].Keywords) != 1 {
		t.Fatalf("saved rules = %#v", saved.Rules)
	}
	if saved.Rules[0].SortOrder != 10 || saved.Rules[0].ID == 0 {
		t.Fatalf("saved rule order/id = %#v", saved.Rules[0])
	}
}

func TestServiceRejectsDuplicateCategoryNames(t *testing.T) {
	service := New(&memoryStore{})
	config := DefaultConfig()
	config.Rules = []Rule{
		{Provider: "openai", CategoryName: "Pro", Keywords: []string{"pro"}, MatchMode: MatchModeContains, Enabled: true},
		{Provider: "openai", CategoryName: "pro", Keywords: []string{"plus"}, MatchMode: MatchModeContains, Enabled: true},
	}
	if _, err := service.Save(context.Background(), config); err == nil {
		t.Fatal("expected duplicate category validation error")
	}
}
