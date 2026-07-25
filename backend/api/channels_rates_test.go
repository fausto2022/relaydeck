package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/rateranking"
	"github.com/fausto2022/relaydeck/backend/storage"
	"github.com/gin-gonic/gin"
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

func TestChannelRateOutputsPreferStoredPlatform(t *testing.T) {
	output := channelRateOutputs([]storage.RateSnapshot{{
		ID: 1, Platform: "anthropic", ModelName: "OpenAI Pro",
	}}, nil)
	if len(output) != 1 || output[0].RankingProvider != "anthropic" {
		t.Fatalf("classification output = %#v", output)
	}
}

func TestListRatesBatchesChannelsAndAppliesRechargeMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTestDB(t)
	channels := storage.NewChannels(db)
	rates := storage.NewRates(db)
	multiplier := 2.0
	items := []storage.Channel{
		{
			Name: "adjusted", Type: storage.ChannelTypeNewAPI, SiteURL: "https://a.example.com",
			Username: "a", PasswordCipher: "x", RechargeMultiplier: &multiplier,
			RechargeMultiplierMode: connector.RechargeMultiplierModeMultiply,
		},
		{
			Name: "plain", Type: storage.ChannelTypeNewAPI, SiteURL: "https://b.example.com",
			Username: "b", PasswordCipher: "x",
		},
	}
	for i := range items {
		if err := channels.Create(&items[i]); err != nil {
			t.Fatalf("create channel: %v", err)
		}
		if _, err := rates.Upsert(&storage.RateSnapshot{
			ChannelID: items[i].ID, ModelName: "default", Ratio: float64(i + 1),
			CompletionRatio: 1, LastSeenAt: time.Now(),
		}); err != nil {
			t.Fatalf("create rate: %v", err)
		}
	}

	router := gin.New()
	api := router.Group("/api")
	registerRates(api, &Deps{Channels: channels, Rates: rates})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/rates?channel_ids=%d,%d,%d", items[1].ID, items[0].ID, items[1].ID),
		nil,
	)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []channelRateOutput `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data) != 2 {
		t.Fatalf("rates = %#v", response.Data)
	}
	if response.Data[0].ChannelID != items[0].ID || response.Data[0].Ratio != 2 {
		t.Fatalf("adjusted rate = %#v", response.Data[0])
	}
	if response.Data[1].ChannelID != items[1].ID || response.Data[1].Ratio != 2 {
		t.Fatalf("plain rate = %#v", response.Data[1])
	}
}
