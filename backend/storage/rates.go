package storage

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const trendCacheTTL = 30 * time.Second

type Rates struct {
	db                *gorm.DB
	balanceTrendMu    sync.Mutex
	costTrendMu       sync.Mutex
	balanceTrendCache map[int]balanceTrendCacheEntry
	costTrendCache    map[int]costTrendCacheEntry
}

type balanceTrendCacheEntry struct {
	loadedAt time.Time
	day      string
	items    []DailyAggregate
}

type costTrendCacheEntry struct {
	loadedAt time.Time
	day      string
	items    []DailyCostAggregate
}

func NewRates(db *gorm.DB) *Rates {
	return &Rates{
		db:                db,
		balanceTrendCache: make(map[int]balanceTrendCacheEntry),
		costTrendCache:    make(map[int]costTrendCacheEntry),
	}
}

var trendNow = time.Now
var trendLocation = loadTrendLocation()

func loadTrendLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}

// ListByChannel 返回渠道当前所有倍率快照。
func (r *Rates) ListByChannel(channelID uint) ([]RateSnapshot, error) {
	var list []RateSnapshot
	if err := r.db.Where("channel_id = ?", channelID).Order("model_name ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListByChannels 返回多个渠道的倍率快照，避免调用方逐渠道查询。
func (r *Rates) ListByChannels(channelIDs []uint) ([]RateSnapshot, error) {
	if len(channelIDs) == 0 {
		return []RateSnapshot{}, nil
	}
	var list []RateSnapshot
	if err := r.db.Where("channel_id IN ?", channelIDs).
		Order("channel_id ASC, model_name ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *Rates) FindByID(channelID, id uint) (*RateSnapshot, error) {
	var item RateSnapshot
	if err := r.db.First(&item, "id = ? AND channel_id = ?", id, channelID).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Upsert 更新或插入倍率快照，返回此前的记录（若有），调用方据此判断是否变化。
func (r *Rates) Upsert(snapshot *RateSnapshot) (*RateSnapshot, error) {
	var prev RateSnapshot
	err := r.db.
		Where("channel_id = ? AND model_name = ?", snapshot.ChannelID, snapshot.ModelName).
		First(&prev).Error
	switch {
	case err == nil:
		old := prev
		prev.Ratio = snapshot.Ratio
		prev.CompletionRatio = snapshot.CompletionRatio
		prev.RemoteGroupID = snapshot.RemoteGroupID
		prev.Description = snapshot.Description
		prev.Platform = snapshot.Platform
		prev.LastSeenAt = snapshot.LastSeenAt
		if err := r.db.Save(&prev).Error; err != nil {
			return nil, err
		}
		return &old, nil
	case err == gorm.ErrRecordNotFound:
		snapshot.FirstSeenAt = snapshot.LastSeenAt
		if err := r.db.Create(snapshot).Error; err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, err
	}
}

func (r *Rates) AppendChange(log *RateChangeLog) error {
	if log.ChangedAt.IsZero() {
		log.ChangedAt = time.Now()
	}
	return r.db.Create(log).Error
}

func (r *Rates) DeleteSnapshot(channelID uint, modelName string) error {
	return r.db.Where("channel_id = ? AND model_name = ?", channelID, modelName).Delete(&RateSnapshot{}).Error
}

// ListChanges 倒序拉取倍率变化日志。channelID 为 0 表示不过滤。
func (r *Rates) ListChanges(channelID uint, limit int) ([]RateChangeLog, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.db.Model(&RateChangeLog{}).Order("changed_at DESC").Limit(limit)
	if channelID != 0 {
		q = q.Where("channel_id = ?", channelID)
	}
	var list []RateChangeLog
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *Rates) ListChangesPage(channelID uint, page, pageSize int) ([]RateChangeLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	q := r.db.Model(&RateChangeLog{})
	if channelID != 0 {
		q = q.Where("channel_id = ?", channelID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []RateChangeLog
	if err := q.Order("changed_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *Rates) AppendBalance(s *BalanceSnapshot) error {
	if s.SampledAt.IsZero() {
		s.SampledAt = time.Now()
	}
	if err := r.db.Create(s).Error; err != nil {
		return err
	}
	r.invalidateBalanceTrend()
	return nil
}

func (r *Rates) AppendCost(s *CostSnapshot) error {
	if s.SampledAt.IsZero() {
		s.SampledAt = time.Now()
	}
	if err := r.db.Create(s).Error; err != nil {
		return err
	}
	r.invalidateCostTrend()
	return nil
}

// ResetStaleTodayCostsAt 清零当天尚无消费采样的渠道，避免停用渠道跨日保留昨日消费。
func (r *Rates) ResetStaleTodayCostsAt(now time.Time) (int64, error) {
	today := dayStart(now)
	timeExpression := "cost_snapshots.sampled_at"
	boundExpression := "?"
	if r.db.Dialector.Name() == "sqlite" {
		timeExpression = "julianday(cost_snapshots.sampled_at)"
		boundExpression = "julianday(?)"
	}
	query := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM cost_snapshots WHERE cost_snapshots.channel_id = channels.id AND %s >= %s)",
		timeExpression,
		boundExpression,
	)
	result := r.db.Model(&Channel{}).
		Where("today_cost IS NOT NULL AND today_cost <> 0").
		Where(query, today).
		Update("today_cost", 0)
	return result.RowsAffected, result.Error
}

// CostHistorySince 返回指定渠道从 since 开始的最近消费采样，结果按时间升序排列。
func (r *Rates) CostHistorySince(channelID uint, since time.Time, limit int) ([]CostSnapshot, error) {
	if limit <= 0 {
		limit = 12000
	}
	var list []CostSnapshot
	if err := r.db.
		Where("channel_id = ? AND sampled_at >= ?", channelID, since).
		Order("sampled_at DESC, id DESC").
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, err
	}
	for left, right := 0, len(list)-1; left < right; left, right = left+1, right-1 {
		list[left], list[right] = list[right], list[left]
	}
	return list, nil
}

// DeleteBalanceSnapshotsBefore 删除 sampled_at < cutoff 的余额快照，返回删除行数。
func (r *Rates) DeleteBalanceSnapshotsBefore(cutoff time.Time) (int64, error) {
	res := r.db.Where("sampled_at < ?", cutoff).Delete(&BalanceSnapshot{})
	if res.Error == nil && res.RowsAffected > 0 {
		r.invalidateBalanceTrend()
	}
	return res.RowsAffected, res.Error
}

// DeleteCostSnapshotsBefore 删除 sampled_at < cutoff 的消费快照，返回删除行数。
func (r *Rates) DeleteCostSnapshotsBefore(cutoff time.Time) (int64, error) {
	res := r.db.Where("sampled_at < ?", cutoff).Delete(&CostSnapshot{})
	if res.Error == nil && res.RowsAffected > 0 {
		r.invalidateCostTrend()
	}
	return res.RowsAffected, res.Error
}

// BalanceHistory 倒序拉取余额历史。
func (r *Rates) BalanceHistory(channelID uint, limit int) ([]BalanceSnapshot, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []BalanceSnapshot
	if err := r.db.
		Where("channel_id = ?", channelID).
		Order("sampled_at DESC").
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DailyAggregate 一天的聚合余额（所有渠道之和）。
type DailyAggregate struct {
	Day     time.Time `json:"day"`
	Balance float64   `json:"balance"`
}

// DailyCostAggregate 一天的聚合消费（所有渠道之和）。
type DailyCostAggregate struct {
	Day  time.Time `json:"day"`
	Cost float64   `json:"cost"`
}

// AggregateBalanceTrend 取最近 N 天的"日内最后一次余额"按渠道之和，作为总余额趋势。
//
// 实现：对每个 (channel_id, day) 取该天最后一次 BalanceSnapshot 的余额，再按 day 求和，
// 然后补齐窗口内缺失的日期。窗口内完全没有采样时返回空数组。
func (r *Rates) AggregateBalanceTrend(days int) ([]DailyAggregate, error) {
	if days <= 0 {
		days = 7
	}
	now := trendNow()
	today := dayStart(now)
	cacheDay := dayKey(today)
	r.balanceTrendMu.Lock()
	defer r.balanceTrendMu.Unlock()
	cached, ok := r.balanceTrendCache[days]
	cacheAge := now.Sub(cached.loadedAt)
	if ok && cached.day == cacheDay && cacheAge >= 0 && cacheAge < trendCacheTTL {
		return append([]DailyAggregate(nil), cached.items...), nil
	}
	since := today.AddDate(0, 0, -(days - 1))

	rows, err := r.aggregateDailyLatest("balance_snapshots", "balance", since, days)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		r.balanceTrendCache[days] = balanceTrendCacheEntry{loadedAt: now, day: cacheDay, items: []DailyAggregate{}}
		return []DailyAggregate{}, nil
	}
	byDay := make(map[string]float64, days)
	for _, row := range rows {
		byDay[dayKey(since.AddDate(0, 0, row.DayIndex))] = row.Total
	}

	out := make([]DailyAggregate, 0, days)
	for day := since; !day.After(today); day = day.AddDate(0, 0, 1) {
		out = append(out, DailyAggregate{Day: day, Balance: byDay[dayKey(day)]})
	}
	r.balanceTrendCache[days] = balanceTrendCacheEntry{loadedAt: now, day: cacheDay, items: append([]DailyAggregate(nil), out...)}
	return out, nil
}

// AggregateCostTrend 取最近 N 天按 Asia/Shanghai 自然日归一化后的渠道消费增量。
func (r *Rates) AggregateCostTrend(days int) ([]DailyCostAggregate, error) {
	if days <= 0 {
		days = 7
	}
	now := trendNow()
	cacheDay := dayKey(dayStart(now))
	r.costTrendMu.Lock()
	defer r.costTrendMu.Unlock()
	cached, ok := r.costTrendCache[days]
	cacheAge := now.Sub(cached.loadedAt)
	if ok && cached.day == cacheDay && cacheAge >= 0 && cacheAge < trendCacheTTL {
		return append([]DailyCostAggregate(nil), cached.items...), nil
	}
	items, err := r.AggregateCostTrendAt(days, now)
	if err != nil {
		return nil, err
	}
	r.costTrendCache[days] = costTrendCacheEntry{loadedAt: now, day: cacheDay, items: append([]DailyCostAggregate(nil), items...)}
	return items, nil
}

func (r *Rates) invalidateBalanceTrend() {
	r.balanceTrendMu.Lock()
	r.balanceTrendCache = make(map[int]balanceTrendCacheEntry)
	r.balanceTrendMu.Unlock()
}

func (r *Rates) invalidateCostTrend() {
	r.costTrendMu.Lock()
	r.costTrendCache = make(map[int]costTrendCacheEntry)
	r.costTrendMu.Unlock()
}

// AggregateCostTrendAt 使用调用方提供的当前时间计算最近 N 天消费趋势，便于业务层保持统一时钟。
func (r *Rates) AggregateCostTrendAt(days int, now time.Time) ([]DailyCostAggregate, error) {
	if days <= 0 {
		days = 7
	}
	today := dayStart(now)
	since := today.AddDate(0, 0, -(days - 1))

	rows, err := r.aggregateDailyCostDeltas(since, days)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []DailyCostAggregate{}, nil
	}
	byDay := make(map[string]float64, days)
	for _, row := range rows {
		byDay[dayKey(since.AddDate(0, 0, row.DayIndex))] += row.Total
	}

	out := make([]DailyCostAggregate, 0, days)
	for day := since; !day.After(today); day = day.AddDate(0, 0, 1) {
		out = append(out, DailyCostAggregate{Day: day, Cost: byDay[dayKey(day)]})
	}
	return out, nil
}

// CurrentDayCostsAt 返回每个渠道按 Asia/Shanghai 自然日归一化后的今日消费。
// 上游晚于北京时间零点归零时，零点前的旧累计值会作为基线；上游真正
// 归零后，归零前后的增量会连续累加，不会重复计算或丢失凌晨消费。
func (r *Rates) CurrentDayCostsAt(now time.Time) (map[uint]float64, error) {
	today := dayStart(now)
	rows, err := r.aggregateDailyCostDeltas(today, 1)
	if err != nil {
		return nil, err
	}
	result := make(map[uint]float64, len(rows))
	for _, row := range rows {
		result[row.ChannelID] = row.Total
	}
	return result, nil
}

type dailyChannelCostRow struct {
	DayIndex  int     `gorm:"column:day_index"`
	ChannelID uint    `gorm:"column:channel_id"`
	Total     float64 `gorm:"column:total"`
}

func (r *Rates) aggregateDailyCostDeltas(since time.Time, days int) ([]dailyChannelCostRow, error) {
	timeExpression := "snapshots.sampled_at"
	boundExpression := "?"
	if r.db.Dialector.Name() == "sqlite" {
		timeExpression = "julianday(snapshots.sampled_at)"
		boundExpression = "julianday(?)"
	}

	var ranges strings.Builder
	args := make([]any, 0, days*2+2)
	for dayIndex := 0; dayIndex < days; dayIndex++ {
		if dayIndex > 0 {
			ranges.WriteString(" UNION ALL ")
		}
		fmt.Fprintf(&ranges, "SELECT %d AS day_index, %s AS start_at, %s AS end_at", dayIndex, boundExpression, boundExpression)
		args = append(args, since.AddDate(0, 0, dayIndex), since.AddDate(0, 0, dayIndex+1))
	}
	args = append(args, since.AddDate(0, 0, -1), since)

	query := fmt.Sprintf(`
WITH day_ranges AS (%s),
target AS (
	SELECT day_ranges.day_index, snapshots.channel_id, snapshots.id,
		snapshots.today_cost AS metric_value, %s AS sampled_order
	FROM cost_snapshots AS snapshots
	INNER JOIN day_ranges
		ON %s >= day_ranges.start_at
		AND %s < day_ranges.end_at
),
baseline_ranked AS (
	SELECT -1 AS day_index, snapshots.channel_id, snapshots.id,
		snapshots.today_cost AS metric_value, %s AS sampled_order,
		ROW_NUMBER() OVER (
			PARTITION BY snapshots.channel_id
			ORDER BY %s DESC, snapshots.id DESC
		) AS baseline_row
	FROM cost_snapshots AS snapshots
	WHERE %s >= %s AND %s < %s
),
relevant AS (
	SELECT day_index, channel_id, id, metric_value, sampled_order
	FROM baseline_ranked WHERE baseline_row = 1
	UNION ALL
	SELECT day_index, channel_id, id, metric_value, sampled_order FROM target
),
sequenced AS (
	SELECT day_index, channel_id, metric_value,
		LAG(metric_value) OVER (
			PARTITION BY channel_id ORDER BY sampled_order ASC, id ASC
		) AS previous_value,
		LAG(day_index) OVER (
			PARTITION BY channel_id ORDER BY sampled_order ASC, id ASC
		) AS previous_day_index
	FROM relevant
),
deltas AS (
	SELECT day_index, channel_id,
		CASE
			WHEN metric_value < 0 THEN 0
			WHEN previous_value IS NULL OR previous_day_index IS NULL OR day_index - previous_day_index > 1 THEN metric_value
			WHEN metric_value >= previous_value THEN metric_value - previous_value
			WHEN previous_value > 0 AND metric_value <= previous_value * 0.9 THEN metric_value
			ELSE 0
		END AS delta
	FROM sequenced
	WHERE day_index >= 0
)
SELECT day_index, channel_id, COALESCE(SUM(delta), 0) AS total
FROM deltas
GROUP BY day_index, channel_id
ORDER BY day_index, channel_id`,
		ranges.String(), timeExpression, timeExpression, timeExpression,
		timeExpression, timeExpression, timeExpression, boundExpression, timeExpression, boundExpression)

	var rows []dailyChannelCostRow
	if err := r.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type dailyAggregateRow struct {
	DayIndex int     `gorm:"column:day_index"`
	Total    float64 `gorm:"column:total"`
}

// aggregateDailyLatest 在数据库中完成“每天每渠道最后一次采样”的筛选和汇总。
// table、valueColumn 仅由上面的固定调用传入，不接受外部输入。
func (r *Rates) aggregateDailyLatest(table, valueColumn string, since time.Time, days int) ([]dailyAggregateRow, error) {
	timeExpression := "snapshots.sampled_at"
	boundExpression := "?"
	if r.db.Dialector.Name() == "sqlite" {
		// SQLite 把 time.Time 保存为带时区的文本；julianday 同时兼容历史 UTC 数据和
		// 当前 Asia/Shanghai 数据，避免直接按字符串比较时跨日错分。
		timeExpression = "julianday(snapshots.sampled_at)"
		boundExpression = "julianday(?)"
	}

	var ranges strings.Builder
	args := make([]any, 0, days*2)
	for dayIndex := 0; dayIndex < days; dayIndex++ {
		if dayIndex > 0 {
			ranges.WriteString(" UNION ALL ")
		}
		fmt.Fprintf(
			&ranges,
			"SELECT %d AS day_index, %s AS start_at, %s AS end_at",
			dayIndex,
			boundExpression,
			boundExpression,
		)
		args = append(args, since.AddDate(0, 0, dayIndex), since.AddDate(0, 0, dayIndex+1))
	}

	query := fmt.Sprintf(`
WITH day_ranges AS (%s),
ranked AS (
	SELECT day_ranges.day_index, snapshots.%s AS metric_value,
		ROW_NUMBER() OVER (
			PARTITION BY day_ranges.day_index, snapshots.channel_id
			ORDER BY %s DESC, snapshots.id DESC
		) AS row_num
	FROM %s AS snapshots
	INNER JOIN day_ranges
		ON %s >= day_ranges.start_at
		AND %s < day_ranges.end_at
)
SELECT day_index, COALESCE(SUM(metric_value), 0) AS total
FROM ranked
WHERE row_num = 1
GROUP BY day_index
ORDER BY day_index`, ranges.String(), valueColumn, timeExpression, table, timeExpression, timeExpression)

	var rows []dailyAggregateRow
	if err := r.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func dayStart(t time.Time) time.Time {
	local := t.In(trendLocation)
	y, m, d := local.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, trendLocation)
}

func dayKey(t time.Time) string {
	return t.Format("2006-01-02")
}
