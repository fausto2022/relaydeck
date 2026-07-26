package mainstation

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/notify"
	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestSendDailyBusinessSummary(t *testing.T) {
	service, db, baseAdmin, _ := newTestService(t)
	now := time.Date(2026, 7, 26, 23, 55, 0, 0, shanghaiLocation())
	service.now = func() time.Time { return now }
	seedProfitCosts(t, db, now, 3)
	admin := &profitAdminClient{
		fakeAdminClient: baseAdmin,
		stats: map[string][]sub2api.AdminGroupUsageStat{
			"2026-07-26": {
				{GroupID: 1, TotalTokens: 1_500_000, ActualCost: profitFloat64(10)},
				{GroupID: 2, TotalTokens: 750_000, ActualCost: profitFloat64(2.5)},
			},
		},
	}
	service.adminFactory = func() adminClient { return admin }
	configureTestStation(t, service)
	ratio := int64(8000)
	if _, err := service.UpdateConfig(context.Background(), ConfigInput{GuaranteedRevenueRatioBP: &ratio}); err != nil {
		t.Fatalf("update guaranteed revenue ratio: %v", err)
	}
	healthTokens := int64(12_630_000)
	if err := service.store.AppendHealthCheck(&storage.MainAccountHealthCheck{
		PoolID: 1, MemberID: 1, RemoteAccountID: 1, Level: "L1", Status: "success",
		TotalTokens: &healthTokens, StartedAt: now.Add(-time.Hour), FinishedAt: now.Add(-time.Hour), CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("append daily health check: %v", err)
	}
	previousDayTokens := int64(5_000_000)
	if err := service.store.AppendHealthCheck(&storage.MainAccountHealthCheck{
		PoolID: 1, MemberID: 1, RemoteAccountID: 1, Level: "L1", Status: "success",
		TotalTokens: &previousDayTokens, StartedAt: now.Add(-24 * time.Hour), FinishedAt: now.Add(-24 * time.Hour), CreatedAt: now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("append previous daily health check: %v", err)
	}

	messages := make(chan struct {
		Event   storage.NotificationEvent `json:"event"`
		Subject string                    `json:"subject"`
		Body    string                    `json:"body"`
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message struct {
			Event   storage.NotificationEvent `json:"event"`
			Subject string                    `json:"subject"`
			Body    string                    `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		messages <- message
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	configBody, err := json.Marshal(map[string]any{"url": server.URL, "method": http.MethodPost})
	if err != nil {
		t.Fatalf("marshal notification config: %v", err)
	}
	configCipher, err := service.cipher.Encrypt(string(configBody))
	if err != nil {
		t.Fatalf("encrypt notification config: %v", err)
	}
	notifications := storage.NewNotifications(db)
	if err := notifications.CreateChannel(&storage.NotificationChannel{
		Name: "webhook", Type: storage.NotifyWebhook, ConfigCipher: configCipher, Subscriptions: "[]", Enabled: true,
	}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	service.SetDispatcher(notify.NewDispatcher(
		notifications,
		service.cipher,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		notify.Policy{SendMaxAttempts: 1},
	))

	service.SendDailyBusinessSummary(context.Background())

	select {
	case message := <-messages:
		if message.Event != storage.EventDailyBusinessSummary {
			t.Fatalf("notification event = %q", message.Event)
		}
		if message.Subject != "[RelayDeck] 每日经营汇总 · 2026-07-26" {
			t.Fatalf("notification subject = %q", message.Subject)
		}
		for _, want := range []string{"今日业务 Token", "2.25M Token", "今日探活 Token", "12.63M Token", "$12.50", "$3.00", "$9.50", "$7.00", "80.00%"} {
			if !strings.Contains(message.Body, want) {
				t.Fatalf("notification body does not contain %q: %s", want, message.Body)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for daily business summary")
	}

	summary, err := service.ProfitSummary(mainStationProfitDays)
	if err != nil {
		t.Fatalf("profit summary: %v", err)
	}
	if summary.TodayRevenue != 12.5 || summary.TodayCost != 3 || summary.TodayProfit != 9.5 || summary.TodayGuaranteedRevenue != 7 {
		t.Fatalf("daily summary profit = %#v", summary)
	}
}
