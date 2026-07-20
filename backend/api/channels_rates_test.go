package api

import (
	"testing"

	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/rateranking"
	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestApplyRechargeMultiplierToRates(t *testing.T) {
	multiplier := 2.0
	list := []storage.RateSnapshot{{Ratio: 0.7, CompletionRatio: 1.4}}
	applyRechargeMultiplierToRates(list, &storage.Channel{
		RechargeMultiplier: &multiplier, RechargeMultiplierMode: connector.RechargeMultiplierModeDivide,
	})
	if list[0].Ratio != 0.35 || list[0].CompletionRatio != 0.7 {
		t.Fatalf("divide rates = %#v", list[0])
	}
	applyRechargeMultiplierToRates(list, &storage.Channel{
		RechargeMultiplier: &multiplier, RechargeMultiplierMode: connector.RechargeMultiplierModeMultiply,
	})
	if list[0].Ratio != 0.7 || list[0].CompletionRatio != 1.4 {
		t.Fatalf("multiply rates = %#v", list[0])
	}
}

func TestChannelRateOutputsIncludeRankingClassification(t *testing.T) {
	config := rateranking.DefaultConfig()
	config.Rules = []rateranking.Rule{{
		Provider: "openai", CategoryName: "Pro", Keywords: []string{"pro"},
		MatchMode: rateranking.MatchModeWord, SortOrder: 10, Enabled: true,
	}}
	output := channelRateOutputs([]storage.RateSnapshot{{
		ID: 1, ModelName: "OpenAI PRO 5H",
	}}, nil, rateranking.NewClassifier(config))
	if len(output) != 1 || output[0].RankingProvider != "openai" || output[0].RankingCategory != "Pro" || !output[0].RankingVisible {
		t.Fatalf("classification output = %#v", output)
	}
}
