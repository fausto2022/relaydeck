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
	for index := range s.rules {
		s.rules[index].ID = uint(index + 1)
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
	if matched.Provider != "openai" || matched.Category != "Pro" || !matched.Visible {
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
