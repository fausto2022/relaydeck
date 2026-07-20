package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fausto2022/relaydeck/backend/rateranking"
	"github.com/fausto2022/relaydeck/backend/storage"
	"github.com/gin-gonic/gin"
)

func TestRateRankingSettingsAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTestDB(t)
	service := rateranking.New(storage.NewRateRankingConfigs(db))
	router := gin.New()
	apiGroup := router.Group("/api")
	registerSettings(apiGroup, &Deps{RateRanking: service})

	body := `{"providers":[{"provider":"openai","include_unmatched":false}],"rules":[{"provider":"openai","category_name":"Pro","keywords":["pro"],"match_mode":"word","sort_order":10,"enabled":true}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings/rate-ranking", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", res.Code, res.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/rate-ranking", nil)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRes.Code, getRes.Body.String())
	}
	var response struct {
		Data rateranking.Config `json:"data"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Rules) != 1 || response.Data.Rules[0].CategoryName != "Pro" {
		t.Fatalf("rules = %#v", response.Data.Rules)
	}
	for _, provider := range response.Data.Providers {
		if provider.Provider == "openai" && provider.IncludeUnmatched {
			t.Fatal("openai unmatched fallback should be disabled")
		}
	}
}
