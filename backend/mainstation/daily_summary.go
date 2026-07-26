package mainstation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/notify"
	"github.com/fausto2022/relaydeck/backend/storage"
)

func (s *Service) SendDailyBusinessSummary(ctx context.Context) {
	if s.dispatcher == nil {
		return
	}
	if err := s.sendDailyBusinessSummary(ctx); err != nil && s.log != nil {
		s.log.Warn("send daily business summary", "err", err)
	}
}

func (s *Service) sendDailyBusinessSummary(ctx context.Context) error {
	_, target, apiKey, err := s.loadAdminTarget()
	if err != nil {
		return err
	}
	client, ok := s.adminFactory().(groupUsageStatsClient)
	if !ok {
		return errors.New("main station client does not support group usage stats")
	}

	sampledAt := s.now()
	day := sampledAt.In(shanghaiLocation()).Format("2006-01-02")
	groups, err := client.ListGroupUsageStats(ctx, sub2api.AdminTarget{
		BaseURL: target.BaseURL,
		APIKey:  apiKey,
	}, day, day)
	if err != nil {
		return fmt.Errorf("fetch daily business summary: %w", err)
	}

	var totalTokens int64
	var revenue float64
	for _, group := range groups {
		totalTokens += group.TotalTokens
		if group.ActualCost == nil {
			return fmt.Errorf("daily business summary group %d has no actual cost", group.GroupID)
		}
		revenue += *group.ActualCost
	}
	if err := s.store.UpsertProfitSnapshot(&storage.MainStationProfitSnapshot{
		Day:       day,
		Revenue:   revenue,
		SampledAt: sampledAt,
	}); err != nil {
		return fmt.Errorf("save daily business summary snapshot: %w", err)
	}

	summary, err := s.ProfitSummary(mainStationProfitDays)
	if err != nil {
		return fmt.Errorf("load daily business summary profit: %w", err)
	}
	localTime := sampledAt.In(shanghaiLocation())
	dayStart := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, localTime.Location())
	healthTokens, err := s.store.SumHealthTokensSince(dayStart)
	if err != nil {
		return fmt.Errorf("load daily health tokens: %w", err)
	}
	body := notify.MarkdownDetails(
		"今日经营数据已完成汇总。",
		notify.Detail("今日业务 Token", formatDailyTokenCount(totalTokens)),
		notify.Detail("今日探活 Token", formatDailyTokenCount(healthTokens)),
		notify.Detail("今日收入", formatDailyMoney(summary.TodayRevenue)),
		notify.Detail("今日成本", formatDailyMoney(summary.TodayCost)),
		notify.Detail("今日利润", formatDailyMoney(summary.TodayProfit)),
		notify.Detail("保底利润", formatDailyMoney(summary.TodayGuaranteedRevenue)),
		notify.Detail("保底折算比例", fmt.Sprintf("%.2f%%", float64(summary.GuaranteedRevenueRatioBP)/100)),
	) + notify.MarkdownNote("统计口径", "按 Asia/Shanghai 自然日统计，金额单位为美元。")
	if err := s.dispatcher.Dispatch(ctx, notify.Message{
		Event:   storage.EventDailyBusinessSummary,
		Subject: "每日经营汇总 · " + day,
		Body:    body,
	}); err != nil {
		return fmt.Errorf("dispatch daily business summary: %w", err)
	}
	return nil
}

func formatDailyTokenCount(tokens int64) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.2fM Token", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.2fK Token", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d Token", tokens)
	}
}

func formatDailyMoney(value float64) string {
	return fmt.Sprintf("$%.2f", value)
}
