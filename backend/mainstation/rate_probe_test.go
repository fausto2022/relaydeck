package mainstation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestQuickTestRateCreatesUsesAndDeletesTemporaryKey(t *testing.T) {
	service, db, _, channels := newTestService(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("probe path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-source-secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "gpt-test" {
			t.Fatalf("model = %#v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test","usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer server.Close()

	channel := createTestChannel(t, db)
	channel.SiteURL = server.URL
	if err := db.Save(channel).Error; err != nil {
		t.Fatalf("save channel: %v", err)
	}
	groupID := int64(301)
	rate := &storage.RateSnapshot{ChannelID: channel.ID, RemoteGroupID: &groupID, ModelName: "source-openai", Ratio: 0.2, LastSeenAt: time.Now()}
	if err := db.Create(rate).Error; err != nil {
		t.Fatalf("create rate: %v", err)
	}

	result, err := service.QuickTestRate(context.Background(), channel.ID, rate.ID, RateQuickTestInput{Platform: "openai", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("quick test rate: %v", err)
	}
	if !result.Usable || !result.Reachable || result.Status != "usable" || result.HTTPStatus != http.StatusOK || result.TotalTokens == nil || *result.TotalTokens != 9 {
		t.Fatalf("quick test result = %#v", result)
	}
	if result.AttemptCount != rateQuickTestAttempts || result.SuccessCount != rateQuickTestAttempts || len(result.Attempts) != rateQuickTestAttempts || requestCount.Load() != rateQuickTestAttempts {
		t.Fatalf("quick test attempts = %#v request_count=%d", result.Attempts, requestCount.Load())
	}
	if result.TemporaryKeyStatus != "deleted" || !strings.HasPrefix(result.TemporaryKeyName, "测试key-") {
		t.Fatalf("temporary key result = %#v", result)
	}
	if len(channels.createdKeys) != 1 || channels.createdKeys[0].GroupID == nil || *channels.createdKeys[0].GroupID != groupID || channels.createdKeys[0].Group != rate.ModelName {
		t.Fatalf("created keys = %#v", channels.createdKeys)
	}
	if len(channels.deletedKeys) != 1 || channels.deletedKeys[0] != 77 {
		t.Fatalf("deleted keys = %#v", channels.deletedKeys)
	}
	var count int64
	if err := db.Model(&storage.MainStationTemporaryAPIKey{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("temporary key records = %d, err=%v", count, err)
	}
}

func TestQuickTestRateGeneratesAndReturnsImagePreview(t *testing.T) {
	service, db, _, _ := newTestService(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("image probe path = %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode image body: %v", err)
		}
		if body["model"] != "gpt-image-test" || body["prompt"] == "" {
			t.Fatalf("image body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer server.Close()

	channel := createTestChannel(t, db)
	channel.SiteURL = server.URL
	if err := db.Save(channel).Error; err != nil {
		t.Fatalf("save channel: %v", err)
	}
	groupID := int64(303)
	rate := &storage.RateSnapshot{ChannelID: channel.ID, RemoteGroupID: &groupID, ModelName: "source-image", Ratio: 0.2, LastSeenAt: time.Now()}
	if err := db.Create(rate).Error; err != nil {
		t.Fatalf("create rate: %v", err)
	}

	result, err := service.QuickTestRate(context.Background(), channel.ID, rate.ID, RateQuickTestInput{Platform: "image", Model: "gpt-image-test"})
	if err != nil {
		t.Fatalf("quick test image rate: %v", err)
	}
	if !result.Usable || result.Protocol != "openai_image" || result.ImageURL != "data:image/png;base64,aW1hZ2U=" {
		t.Fatalf("image quick test result = %#v", result)
	}
	if result.AttemptCount != 1 || result.SuccessCount != 1 || requestCount.Load() != 1 {
		t.Fatalf("image attempts = %#v request_count=%d", result.Attempts, requestCount.Load())
	}
}

func TestQuickTestResultRequiresEveryAttemptToSucceed(t *testing.T) {
	result := quickTestResult([]probeExecution{
		{Status: "success", HTTPStatus: http.StatusOK, Protocol: "openai_chat", Model: "gpt-test", LatencyMS: 100, TTFBMS: 80},
		{Status: "success", HTTPStatus: http.StatusOK, Protocol: "openai_chat", Model: "gpt-test", LatencyMS: 200, TTFBMS: 160},
		{Status: "failure", HTTPStatus: http.StatusTooManyRequests, Protocol: "openai_chat", Model: "gpt-test", LatencyMS: 300, TTFBMS: 240, ErrorClass: "rate_limited"},
	}, "测试key-ABC123", nil, time.Now())
	if result.Usable || !result.Reachable || result.Status != "reachable" || result.SuccessCount != 2 || result.AttemptCount != 3 {
		t.Fatalf("partial quick test result = %#v", result)
	}
	if result.LatencyMS != 150 || result.TTFBMS != 120 || result.HTTPStatus != http.StatusTooManyRequests || !strings.Contains(result.Message, "连接不稳定") {
		t.Fatalf("partial quick test summary = %#v", result)
	}
}

func TestQuickTestRateRetriesFailedTemporaryKeyCleanup(t *testing.T) {
	service, db, _, channels := newTestService(t)
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	channel := createTestChannel(t, db)
	channel.SiteURL = server.URL
	if err := db.Save(channel).Error; err != nil {
		t.Fatalf("save channel: %v", err)
	}
	groupID := int64(302)
	rate := &storage.RateSnapshot{ChannelID: channel.ID, RemoteGroupID: &groupID, ModelName: "source-openai", Ratio: 0.3, LastSeenAt: now}
	if err := db.Create(rate).Error; err != nil {
		t.Fatalf("create rate: %v", err)
	}
	channels.deleteKeyErr = errors.New("temporary delete failure")
	result, err := service.QuickTestRate(context.Background(), channel.ID, rate.ID, RateQuickTestInput{Platform: "openai", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("quick test rate: %v", err)
	}
	if !result.Usable || result.TemporaryKeyStatus != "pending" || result.CleanupError == "" {
		t.Fatalf("cleanup pending result = %#v", result)
	}
	var count int64
	if err := db.Model(&storage.MainStationTemporaryAPIKey{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("temporary key records = %d, err=%v", count, err)
	}

	channels.deleteKeyErr = nil
	now = now.Add(2 * time.Minute)
	service.CleanupTemporaryAPIKeys(context.Background())
	if err := db.Model(&storage.MainStationTemporaryAPIKey{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("temporary key records after retry = %d, err=%v", count, err)
	}
	if len(channels.deletedKeys) != 2 {
		t.Fatalf("cleanup attempts = %#v", channels.deletedKeys)
	}
}

func TestTemporaryAPIKeyRequestUsesNewAPIGroupName(t *testing.T) {
	expiresAt := time.Date(2026, 7, 19, 12, 10, 0, 0, time.UTC)
	request, err := temporaryAPIKeyRequest(
		&storage.Channel{Type: storage.ChannelTypeNewAPI},
		&storage.RateSnapshot{ModelName: "default"},
		"测试key-ABC123",
		expiresAt,
	)
	if err != nil {
		t.Fatalf("temporary api key request: %v", err)
	}
	if request.Group != "default" || request.GroupID != nil || request.ExpiredTime == nil || *request.ExpiredTime != expiresAt.Unix() || request.RemainQuota == nil || *request.RemainQuota <= 0 {
		t.Fatalf("newapi temporary request = %#v", request)
	}
}
