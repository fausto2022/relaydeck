package storage

import (
	"context"
	"testing"
)

func TestRateRankingConfigsReplacePersistsDisabledFallback(t *testing.T) {
	db := openTestDB(t)
	store := NewRateRankingConfigs(db)
	keywords, err := EncodeRateRankingKeywords([]string{"pro", "专业"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(context.Background(), []RateRankingProviderSetting{
		{Provider: "openai", IncludeUnmatched: false},
	}, []RateRankingCategoryRule{
		{Provider: "openai", CategoryName: "Pro", KeywordsJSON: keywords, MatchMode: "contains", SortOrder: 10, Enabled: true},
	}); err != nil {
		t.Fatalf("replace ranking config: %v", err)
	}

	providers, rules, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list ranking config: %v", err)
	}
	if len(providers) != 1 || providers[0].IncludeUnmatched {
		t.Fatalf("providers = %#v", providers)
	}
	if len(rules) != 1 || rules[0].CategoryName != "Pro" || !rules[0].Enabled {
		t.Fatalf("rules = %#v", rules)
	}
	decoded, err := DecodeRateRankingKeywords(rules[0].KeywordsJSON)
	if err != nil || len(decoded) != 2 || decoded[1] != "专业" {
		t.Fatalf("decoded keywords = %#v, err=%v", decoded, err)
	}
}

func TestRateRankingConfigsReplacePreservesRuleIDs(t *testing.T) {
	db := openTestDB(t)
	store := NewRateRankingConfigs(db)
	keywords, err := EncodeRateRankingKeywords([]string{"pro"})
	if err != nil {
		t.Fatal(err)
	}
	initial := []RateRankingCategoryRule{
		{Provider: "openai", CategoryName: "First", KeywordsJSON: keywords, MatchMode: "contains", SortOrder: 10, Enabled: true},
		{Provider: "openai", CategoryName: "Second", KeywordsJSON: keywords, MatchMode: "contains", SortOrder: 20, Enabled: true},
	}
	if err := store.Replace(context.Background(), nil, initial); err != nil {
		t.Fatalf("replace initial rules: %v", err)
	}
	_, saved, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list initial rules: %v", err)
	}
	secondID := saved[1].ID
	if err := store.Replace(context.Background(), nil, []RateRankingCategoryRule{
		{ID: secondID, Provider: "openai", CategoryName: "Renamed", KeywordsJSON: keywords, MatchMode: "contains", SortOrder: 10, Enabled: true},
	}); err != nil {
		t.Fatalf("replace renamed rule: %v", err)
	}
	_, saved, err = store.List(context.Background())
	if err != nil || len(saved) != 1 || saved[0].ID != secondID || saved[0].CategoryName != "Renamed" {
		t.Fatalf("saved rules = %#v, err=%v", saved, err)
	}
}
