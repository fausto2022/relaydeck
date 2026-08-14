package mainstation

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/storage"
	"gorm.io/gorm"
)

type profitAdminClient struct {
	*fakeAdminClient
	stats map[string][]sub2api.AdminGroupUsageStat
	calls []string
}

func TestTodayGroupUsageReturnsCurrentDayStats(t *testing.T) {
	service, _, baseAdmin, _ := newTestService(t)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, shanghaiLocation())
	service.now = func() time.Time { return now }
	admin := &profitAdminClient{
		fakeAdminClient: baseAdmin,
		stats: map[string][]sub2api.AdminGroupUsageStat{
			"2026-07-18": {{GroupID: 3, GroupName: "高用量", Requests: 12, TotalTokens: 3456, ActualCost: profitFloat64(4.5)}},
		},
	}
	service.adminFactory = func() adminClient { return admin }
	configureTestStation(t, service)

	items, err := service.TodayGroupUsage(context.Background())
	if err != nil {
		t.Fatalf("today group usage: %v", err)
	}
	if len(items) != 1 || items[0].GroupID != 3 || items[0].GroupName != "高用量" || items[0].Requests != 12 || items[0].TotalTokens != 3456 || items[0].ActualCost == nil || *items[0].ActualCost != 4.5 {
		t.Fatalf("today group usage = %#v", items)
	}
	if len(admin.calls) != 1 || admin.calls[0] != "2026-07-18:2026-07-18" {
		t.Fatalf("group usage calls = %#v", admin.calls)
	}
}

func (f *profitAdminClient) ListGroupUsageStats(_ context.Context, _ sub2api.AdminTarget, startDate, endDate string) ([]sub2api.AdminGroupUsageStat, error) {
	f.calls = append(f.calls, startDate+":"+endDate)
	return append([]sub2api.AdminGroupUsageStat(nil), f.stats[startDate]...), nil
}

func TestSyncBackfillsAndUpdatesProfitSummary(t *testing.T) {
	service, db, baseAdmin, _ := newTestService(t)
	location := shanghaiLocation()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, location)
	service.now = func() time.Time { return now }
	channel := seedProfitCosts(t, db, now, 3)
	admin := &profitAdminClient{fakeAdminClient: baseAdmin, stats: map[string][]sub2api.AdminGroupUsageStat{}}
	for offset := 0; offset < mainStationProfitDays; offset++ {
		day := now.AddDate(0, 0, -offset).Format("2006-01-02")
		admin.stats[day] = []sub2api.AdminGroupUsageStat{{
			GroupID: 1, ActualCost: profitFloat64(10), AccountCost: profitFloat64(600),
		}}
	}
	service.adminFactory = func() adminClient { return admin }
	configureTestStation(t, service)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync main station: %v", err)
	}
	summary, err := service.ProfitSummary(7)
	if err != nil {
		t.Fatalf("profit summary: %v", err)
	}
	if !summary.Available || !summary.TodayAvailable || !summary.Complete || summary.SampledDays != 7 {
		t.Fatalf("profit availability = %#v", summary)
	}
	if summary.TodayProfit != 7 || summary.SevenDayRevenue != 70 || summary.SevenDayCost != 21 || summary.SevenDayProfit != 49 {
		t.Fatalf("profit summary = %#v", summary)
	}
	if len(admin.calls) != 7 {
		t.Fatalf("initial profit calls = %v", admin.calls)
	}

	today := now.Format("2006-01-02")
	admin.stats[today] = []sub2api.AdminGroupUsageStat{{
		GroupID: 1, ActualCost: profitFloat64(12), AccountCost: profitFloat64(500),
	}}
	if err := storage.NewChannels(db).UpdateCosts(channel.ID, 4, 4); err != nil {
		t.Fatalf("update current upstream cost: %v", err)
	}
	if err := service.rates.AppendCost(&storage.CostSnapshot{ChannelID: channel.ID, TodayCost: 4, SampledAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("append refreshed cost: %v", err)
	}
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("refresh main station: %v", err)
	}
	summary, err = service.ProfitSummary(7)
	if err != nil {
		t.Fatalf("refreshed profit summary: %v", err)
	}
	if len(admin.calls) != 8 || summary.TodayProfit != 8 || summary.SevenDayProfit != 50 {
		t.Fatalf("refreshed summary = %#v, calls = %v", summary, admin.calls)
	}
}

func TestSyncUsesActualCostWithoutAccountCost(t *testing.T) {
	service, db, baseAdmin, _ := newTestService(t)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, shanghaiLocation())
	service.now = func() time.Time { return now }
	seedProfitCosts(t, db, now, 3)
	admin := &profitAdminClient{
		fakeAdminClient: baseAdmin,
		stats: map[string][]sub2api.AdminGroupUsageStat{
			now.Format("2006-01-02"): {{GroupID: 1, ActualCost: profitFloat64(10)}},
		},
	}
	service.adminFactory = func() adminClient { return admin }
	configureTestStation(t, service)
	guaranteedRevenueRatio := int64(8000)
	if _, err := service.UpdateConfig(context.Background(), ConfigInput{GuaranteedRevenueRatioBP: &guaranteedRevenueRatio}); err != nil {
		t.Fatalf("update guaranteed revenue ratio: %v", err)
	}
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync main station: %v", err)
	}
	summary, err := service.ProfitSummary(7)
	if err != nil {
		t.Fatalf("profit summary: %v", err)
	}
	if !summary.Available || summary.TodayRevenue != 10 || summary.TodayGuaranteedRevenue != 5 || summary.GuaranteedRevenueRatioBP != guaranteedRevenueRatio || summary.TodayCost != 3 || summary.TodayProfit != 7 {
		t.Fatalf("profit should use actual revenue and converted upstream cost: %#v", summary)
	}
}

func TestProfitSummaryIgnoresStaleDisabledChannelCostAfterMidnight(t *testing.T) {
	service, db, _, _ := newTestService(t)
	now := time.Date(2026, 8, 2, 0, 15, 0, 0, shanghaiLocation())
	service.now = func() time.Time { return now }
	staleCost := 12.7289
	channel := &storage.Channel{
		Name: "disabled-source", Type: storage.ChannelTypeSub2API, SiteURL: "https://source.example.com",
		Username: "user", PasswordCipher: "cipher", MonitorEnabled: false, TodayCost: &staleCost,
	}
	if err := storage.NewChannels(db).Create(channel); err != nil {
		t.Fatalf("create disabled source: %v", err)
	}
	if err := storage.NewRates(db).AppendCost(&storage.CostSnapshot{
		ChannelID: channel.ID, TodayCost: staleCost, SampledAt: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("append yesterday cost: %v", err)
	}
	if err := storage.NewMainStationStore(db).UpsertProfitSnapshot(&storage.MainStationProfitSnapshot{
		Day: now.Format("2006-01-02"), Revenue: 20.21, SampledAt: now,
	}); err != nil {
		t.Fatalf("save profit snapshot: %v", err)
	}

	summary, err := service.ProfitSummary(7)
	if err != nil {
		t.Fatalf("profit summary: %v", err)
	}
	if summary.TodayCost != 0 || summary.TodayProfit != 20.21 {
		t.Fatalf("stale cost should be ignored after midnight: %#v", summary)
	}
}

func TestProfitSummaryAccumulatesCostAcrossDelayedUpstreamReset(t *testing.T) {
	service, db, _, _ := newTestService(t)
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, shanghaiLocation())
	service.now = func() time.Time { return now }
	channel := &storage.Channel{
		Name: "delayed-reset", Type: storage.ChannelTypeSub2API, SiteURL: "https://source.example.com",
		Username: "user", PasswordCipher: "cipher", MonitorEnabled: true,
	}
	if err := storage.NewChannels(db).Create(channel); err != nil {
		t.Fatalf("create delayed reset source: %v", err)
	}
	rates := storage.NewRates(db)
	for _, snapshot := range []storage.CostSnapshot{
		{ChannelID: channel.ID, TodayCost: 33.0, SampledAt: now.Add(-10*time.Hour - 5*time.Minute)},
		{ChannelID: channel.ID, TodayCost: 33.1, SampledAt: now.Add(-9*time.Hour - 55*time.Minute)},
		{ChannelID: channel.ID, TodayCost: 33.4, SampledAt: now.Add(-4 * time.Hour)},
		{ChannelID: channel.ID, TodayCost: 0.2, SampledAt: now.Add(-2 * time.Hour)},
		{ChannelID: channel.ID, TodayCost: 0.5, SampledAt: now},
	} {
		snapshot := snapshot
		if err := rates.AppendCost(&snapshot); err != nil {
			t.Fatalf("append delayed reset cost: %v", err)
		}
	}
	service.rates = rates
	if err := storage.NewMainStationStore(db).UpsertProfitSnapshot(&storage.MainStationProfitSnapshot{
		Day: now.Format("2006-01-02"), Revenue: 2, SampledAt: now,
	}); err != nil {
		t.Fatalf("save delayed reset profit snapshot: %v", err)
	}

	summary, err := service.ProfitSummary(7)
	if err != nil {
		t.Fatalf("profit summary: %v", err)
	}
	if math.Abs(summary.TodayCost-0.9) > 0.000001 || math.Abs(summary.TodayProfit-1.1) > 0.000001 {
		t.Fatalf("delayed reset profit summary = %#v", summary)
	}
}

func profitFloat64(value float64) *float64 { return &value }

func seedProfitCosts(t *testing.T, db *gorm.DB, now time.Time, dailyCost float64) *storage.Channel {
	t.Helper()
	channel := &storage.Channel{
		Name: "profit-source", Type: storage.ChannelTypeSub2API, SiteURL: "https://source.example.com",
		Username: "user", PasswordCipher: "cipher", MonitorEnabled: true, TodayCost: &dailyCost,
	}
	if err := storage.NewChannels(db).Create(channel); err != nil {
		t.Fatalf("create profit source channel: %v", err)
	}
	rates := storage.NewRates(db)
	for offset := 0; offset < mainStationProfitDays; offset++ {
		if err := rates.AppendCost(&storage.CostSnapshot{
			ChannelID: channel.ID, TodayCost: 0, SampledAt: now.AddDate(0, 0, -offset).Add(-11 * time.Hour),
		}); err != nil {
			t.Fatalf("append profit cost reset: %v", err)
		}
		if err := rates.AppendCost(&storage.CostSnapshot{
			ChannelID: channel.ID, TodayCost: dailyCost, SampledAt: now.AddDate(0, 0, -offset),
		}); err != nil {
			t.Fatalf("append profit cost snapshot: %v", err)
		}
	}
	return channel
}
