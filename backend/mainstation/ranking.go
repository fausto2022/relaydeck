package mainstation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/storage"
)

type schedulingRankSignal struct {
	MemberID        uint
	HealthBand      int
	Preferred       bool
	Priority        int
	CurrentPriority int
	Score           int
	CostKnown       bool
	CostMicros      int64
	SuccessBucket   int
	LatencyBucket   int
	SuccessRate     *float64
	P95LatencyMS    *int64
	SampleBand      int
	Enabled         bool
}

const schedulingPriorityStep = 10

const (
	schedulingScoreHysteresis          = 2
	schedulingStabilityScoreHysteresis = 4
)

func normalizeSchedulingPriority(priority int) int {
	if priority > 0 {
		return priority
	}
	return 1
}

func automaticLoadFactor(concurrency int) int {
	if concurrency <= 0 {
		return 1
	}
	return concurrency
}

func (s *Service) poolSchedulingPriorities(poolID uint) (map[uint]int, error) {
	pool, err := s.store.FindPool(poolID)
	if err != nil {
		return nil, err
	}
	members, err := s.store.ListMembers(poolID)
	if err != nil {
		return nil, err
	}
	priorities, _, err := s.poolSchedulingPrioritiesFor(pool, members)
	return priorities, err
}

func (s *Service) poolSchedulingPrioritiesFor(
	pool *storage.MainAccountPool,
	members []storage.MainAccountPoolMember,
) (map[uint]int, map[int64]*storage.MainStationAccountSnapshot, error) {
	now := s.now()
	memberIDs := make([]uint, 0, len(members))
	for i := range members {
		memberIDs = append(memberIDs, members[i].ID)
	}
	recentByMember, err := s.store.ListRecentHealthChecksByMember(memberIDs, 20)
	if err != nil {
		return nil, nil, err
	}
	aggregates, err := s.store.HealthAggregates(memberIDs, now)
	if err != nil {
		return nil, nil, err
	}
	snapshots, err := s.store.ListAllAccountSnapshots(true)
	if err != nil {
		return nil, nil, err
	}
	snapshotsByRemote := make(map[int64]*storage.MainStationAccountSnapshot, len(snapshots))
	for i := range snapshots {
		snapshot := &snapshots[i]
		snapshotsByRemote[snapshot.RemoteAccountID] = snapshot
	}
	locks, err := s.store.ListAllActiveGuardLocks()
	if err != nil {
		return nil, nil, err
	}
	lockedRemoteIDs := make(map[int64]struct{}, len(locks))
	for i := range locks {
		lockedRemoteIDs[locks[i].RemoteAccountID] = struct{}{}
	}
	signals := make([]schedulingRankSignal, 0, len(members))
	for i := range members {
		member := &members[i]
		stats := buildMemberHealthStats(member, recentByMember[member.ID], aggregates[member.ID])
		costKnown := validCostSnapshot(member, now)
		costMicros := int64(0)
		if costKnown {
			costMicros = *member.LastCostMicros
		}
		currentPriority := 0
		remoteSchedulable := false
		remoteActive := false
		locksClear := false
		if member.RemoteAccountID != nil {
			if snapshot := snapshotsByRemote[*member.RemoteAccountID]; snapshot != nil {
				currentPriority = normalizeRemotePriority(snapshot.Priority)
				remoteSchedulable = snapshot.Schedulable && !snapshot.Missing
				remoteActive = strings.EqualFold(snapshot.Status, "active")
			}
			_, locked := lockedRemoteIDs[*member.RemoteAccountID]
			locksClear = !locked
		}
		healthBand := schedulingHealthBand(member)
		successBucket := schedulingSuccessBucket(stats.Recent20SuccessRate)
		latencyBucket := schedulingLatencyBucket(stats.P95LatencyMS)
		successRate := stats.Recent20SuccessRate
		p95LatencyMS := stats.P95LatencyMS
		sampleBand := 0
		if member.HealthEnabled {
			switch sampleCount := len(recentByMember[member.ID]); {
			case sampleCount == 0:
				sampleBand = 2
			case sampleCount < stabilityMinimumSamples:
				sampleBand = 1
			}
		}
		if !member.HealthEnabled {
			healthBand = 0
			successBucket = 0
			latencyBucket = 0
			successRate = nil
			p95LatencyMS = nil
			sampleBand = 2
		}
		signals = append(signals, schedulingRankSignal{
			MemberID:        member.ID,
			HealthBand:      healthBand,
			Preferred:       member.Preferred,
			Priority:        normalizeSchedulingPriority(member.Priority),
			CurrentPriority: currentPriority,
			CostKnown:       costKnown,
			CostMicros:      costMicros,
			SuccessBucket:   successBucket,
			LatencyBucket:   latencyBucket,
			SuccessRate:     successRate,
			P95LatencyMS:    p95LatencyMS,
			SampleBand:      sampleBand,
			Enabled: pool.Enabled && member.Enabled && remoteSchedulable && remoteActive && locksClear &&
				member.BindingStatus != "invalid" && member.BindingStatus != "orphaned" &&
				(!member.HealthEnabled || (member.LastHealthStatus != "unhealthy" && member.Status != "quarantined")),
		})
	}
	return rankSchedulingSignals(signals, pool.RateSortDirection), snapshotsByRemote, nil
}

func rankSchedulingSignals(signals []schedulingRankSignal, rateSortDirection string) map[uint]int {
	stabilityMode := strings.EqualFold(rateSortDirection, "stability")
	costPenalties := schedulingCostPenalties(signals, rateSortDirection)
	hysteresis := schedulingScoreHysteresis
	if stabilityMode {
		hysteresis = schedulingStabilityScoreHysteresis
	}
	for i := range signals {
		if stabilityMode {
			signals[i].Score = schedulingStabilityScore(signals[i]) + costPenalties[signals[i].MemberID]
			continue
		}
		signals[i].Score = signals[i].Priority + schedulingSuccessPenalty(signals[i].SuccessBucket) +
			schedulingLatencyPenalty(signals[i].LatencyBucket) + costPenalties[signals[i].MemberID]
	}
	sort.SliceStable(signals, func(i, j int) bool {
		left, right := signals[i], signals[j]
		switch {
		case left.Enabled != right.Enabled:
			return left.Enabled
		case left.HealthBand != right.HealthBand:
			return left.HealthBand < right.HealthBand
		case left.Preferred != right.Preferred:
			return left.Preferred
		case stabilityMode && left.SampleBand != right.SampleBand:
			return left.SampleBand < right.SampleBand
		case left.CurrentPriority > 0 && right.CurrentPriority > 0 && left.CurrentPriority != right.CurrentPriority:
			return left.CurrentPriority < right.CurrentPriority
		case left.Score != right.Score:
			return left.Score < right.Score
		case left.CurrentPriority != right.CurrentPriority:
			if left.CurrentPriority == 0 {
				return false
			}
			if right.CurrentPriority == 0 {
				return true
			}
			return left.CurrentPriority < right.CurrentPriority
		case left.Priority != right.Priority:
			return left.Priority < right.Priority
		default:
			return left.MemberID < right.MemberID
		}
	})
	for index := 1; index < len(signals); index++ {
		for current := index; current > 0; current-- {
			left, right := signals[current-1], signals[current]
			if !sameSchedulingRankBand(left, right) || right.Score+hysteresis >= left.Score {
				break
			}
			signals[current-1], signals[current] = signals[current], signals[current-1]
		}
	}
	return assignSparseSchedulingPriorities(signals)
}

const (
	stabilityMinimumSamples            = 5
	stabilityUnknownPenalty            = 1_000
	stabilityInsufficientSamplePenalty = 250
)

func schedulingStabilityScore(signal schedulingRankSignal) int {
	score := signal.Priority
	// 连通率每下降 1 个百分点约等于 P95 延迟增加 2 秒，避免速度掩盖明显的失败率。
	if signal.SuccessRate != nil {
		failureRate := 100 - *signal.SuccessRate
		if failureRate < 0 {
			failureRate = 0
		}
		score += int(failureRate*4 + 0.5)
	} else {
		score += schedulingSuccessPenalty(signal.SuccessBucket)
	}
	if signal.P95LatencyMS != nil {
		latency := *signal.P95LatencyMS
		if latency < 0 {
			latency = 0
		}
		score += int(latency / 500)
	} else {
		score += schedulingLatencyPenalty(signal.LatencyBucket)
	}
	switch signal.SampleBand {
	case 1:
		score += stabilityInsufficientSamplePenalty
	case 2:
		score += stabilityUnknownPenalty
	}
	return score
}

func sameSchedulingRankBand(left, right schedulingRankSignal) bool {
	return left.Enabled == right.Enabled && left.HealthBand == right.HealthBand && left.Preferred == right.Preferred
}

func schedulingCostPenalties(signals []schedulingRankSignal, direction string) map[uint]int {
	known := make([]schedulingRankSignal, 0, len(signals))
	for _, signal := range signals {
		if signal.CostKnown {
			known = append(known, signal)
		}
	}
	sort.SliceStable(known, func(i, j int) bool {
		if direction == "desc" {
			return known[i].CostMicros > known[j].CostMicros
		}
		return known[i].CostMicros < known[j].CostMicros
	})
	penalties := make(map[uint]int, len(signals))
	unknownPenalty := 12
	if direction == "stability" {
		unknownPenalty = 4
	}
	for _, signal := range signals {
		penalties[signal.MemberID] = unknownPenalty
	}
	lastCost := int64(0)
	lastBucket := 0
	for index, signal := range known {
		bucket := lastBucket
		if index == 0 || signal.CostMicros != lastCost {
			bucket = index * 4 / maxInt(len(known), 1)
			lastCost = signal.CostMicros
			lastBucket = bucket
		}
		if direction == "stability" {
			penalties[signal.MemberID] = bucket
		} else {
			penalties[signal.MemberID] = bucket * 3
		}
	}
	return penalties
}

func schedulingSuccessPenalty(bucket int) int {
	switch bucket {
	case 0:
		return 0
	case 1:
		return 2
	case 2:
		return 6
	case 3:
		return 15
	default:
		return 8
	}
}

func schedulingLatencyPenalty(bucket int) int {
	switch bucket {
	case 0:
		return 0
	case 1:
		return 2
	case 2:
		return 5
	case 3:
		return 10
	default:
		return 4
	}
}

func assignSparseSchedulingPriorities(ordered []schedulingRankSignal) map[uint]int {
	priorities := make(map[uint]int, len(ordered))
	if len(ordered) == 0 {
		return priorities
	}
	anchors := longestIncreasingPrioritySubsequence(ordered)
	if len(anchors) == 0 {
		return assignFullSparsePriorities(ordered)
	}
	values := make([]int, len(ordered))
	for _, index := range anchors {
		values[index] = normalizeRemotePriority(ordered[index].CurrentPriority)
	}
	start := 0
	left := 0
	for _, anchor := range anchors {
		count := anchor - start
		right := values[anchor]
		if count > 0 {
			if right-left-1 < count {
				return assignFullSparsePriorities(ordered)
			}
			step := (right - left) / (count + 1)
			if step <= 0 {
				return assignFullSparsePriorities(ordered)
			}
			for index := 0; index < count; index++ {
				values[start+index] = left + step*(index+1)
			}
		}
		start = anchor + 1
		left = right
	}
	for index := start; index < len(ordered); index++ {
		values[index] = left + (index-start+1)*schedulingPriorityStep
	}
	if sparsePrioritiesWouldGrow(ordered, values) {
		return assignFullSparsePriorities(ordered)
	}
	for index, signal := range ordered {
		priorities[signal.MemberID] = values[index]
	}
	return priorities
}

func longestIncreasingPrioritySubsequence(ordered []schedulingRankSignal) []int {
	bestAt := make([]int, len(ordered))
	previous := make([]int, len(ordered))
	best := -1
	bestLength := 0
	for index := range ordered {
		if ordered[index].CurrentPriority <= 0 {
			previous[index] = -1
			continue
		}
		length := 1
		previous[index] = -1
		for prior := 0; prior < index; prior++ {
			if ordered[prior].CurrentPriority > 0 && ordered[prior].CurrentPriority < ordered[index].CurrentPriority && bestAt[prior] >= length {
				length = bestAt[prior] + 1
				previous[index] = prior
			}
		}
		bestAt[index] = length
		if length > bestLength {
			bestLength = length
			best = index
		}
	}
	result := make([]int, bestLength)
	for index := bestLength - 1; index >= 0; index-- {
		result[index] = best
		best = previous[best]
	}
	return result
}

func sparsePrioritiesWouldGrow(ordered []schedulingRankSignal, values []int) bool {
	maxCurrent := 0
	newAccounts := 0
	for _, signal := range ordered {
		if signal.CurrentPriority > maxCurrent {
			maxCurrent = signal.CurrentPriority
		}
		if signal.CurrentPriority <= 0 {
			newAccounts++
		}
	}
	if maxCurrent == 0 || len(values) == 0 {
		return false
	}
	maxAllowed := maxCurrent + newAccounts*schedulingPriorityStep
	return values[len(values)-1] > maxAllowed
}

func assignFullSparsePriorities(ordered []schedulingRankSignal) map[uint]int {
	result := make(map[uint]int, len(ordered))
	for index, signal := range ordered {
		result[signal.MemberID] = (index + 1) * schedulingPriorityStep
	}
	return result
}

func normalizeRemotePriority(priority int) int {
	if priority > 0 {
		return priority
	}
	return 0
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func schedulingHealthBand(member *storage.MainAccountPoolMember) int {
	if member == nil || !member.Enabled || member.BindingStatus == "invalid" || member.BindingStatus == "orphaned" {
		return 4
	}
	switch strings.ToLower(strings.TrimSpace(member.LastHealthStatus)) {
	case "healthy":
		return 0
	case "", "unknown", "pending":
		return 1
	case "degraded", "rate_limited":
		return 2
	case "unhealthy", "quarantined":
		return 3
	default:
		return 4
	}
}

func schedulingSuccessBucket(rate *float64) int {
	if rate == nil {
		return 4
	}
	switch {
	case *rate >= 99:
		return 0
	case *rate >= 95:
		return 1
	case *rate >= 80:
		return 2
	default:
		return 3
	}
}

func schedulingLatencyBucket(p95 *int64) int {
	if p95 == nil {
		return 4
	}
	switch {
	case *p95 <= 2_000:
		return 0
	case *p95 <= 5_000:
		return 1
	case *p95 <= 10_000:
		return 2
	default:
		return 3
	}
}

func (s *Service) markPoolRankingDirty(poolID uint) error {
	if poolID == 0 {
		return errors.New("invalid account pool id")
	}
	return s.store.MarkPoolRankingDirty(poolID, s.now())
}

func validatePoolRankingInterval(value int) error {
	if value == 0 {
		return nil
	}
	return validateRankingInterval(value)
}

func effectivePoolRankingInterval(pool *storage.MainAccountPool, globalSeconds int) time.Duration {
	seconds := normalizedRankingInterval(globalSeconds)
	if pool != nil && pool.RankingIntervalSeconds != 0 {
		seconds = normalizedRankingInterval(pool.RankingIntervalSeconds)
	}
	return time.Duration(seconds) * time.Second
}

func (s *Service) RunDueRankings(ctx context.Context) {
	config, err := s.store.GetConfig()
	if err != nil || !config.Enabled {
		return
	}
	pools, err := s.store.ListAllPools()
	if err != nil {
		if s.log != nil {
			s.log.Warn("list due main station rankings", "err", err)
		}
		return
	}
	now := s.now()
	for i := range pools {
		pool := &pools[i]
		if pool.RankingDirtyAt == nil && pool.LastRankingAt != nil {
			continue
		}
		if pool.LastRankingAt != nil && now.Before(pool.LastRankingAt.Add(effectivePoolRankingInterval(pool, config.RankingIntervalSeconds))) {
			continue
		}
		if err := s.ReconcilePoolRanking(ctx, pool.ID, "scheduler"); err != nil && s.log != nil {
			s.log.Warn("scheduled main station ranking", "err", err, "pool_id", pool.ID)
		}
	}
}

func (s *Service) RecalculateGroupRanking(ctx context.Context, groupID uint) error {
	poolID, err := s.GroupPoolID(groupID)
	if err != nil {
		return err
	}
	if err := s.markPoolRankingDirty(poolID); err != nil {
		return err
	}
	return s.ReconcilePoolRanking(ctx, poolID, "manual")
}

func (s *Service) ReconcilePoolRanking(ctx context.Context, poolID uint, source string) error {
	value, _ := s.rankingLocks.LoadOrStore(poolID, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()
	startedAt := s.now()
	err := s.reconcilePoolRanking(ctx, poolID, source)
	finishedAt := s.now()
	errText := ""
	if err != nil {
		errText = sanitizeText(err.Error())
	}
	if completeErr := s.store.CompletePoolRanking(poolID, startedAt, finishedAt, errText); completeErr != nil {
		err = errors.Join(err, completeErr)
	}
	return err
}

func (s *Service) reconcilePoolRanking(ctx context.Context, poolID uint, source string) error {
	pool, err := s.store.FindPool(poolID)
	if err != nil {
		return err
	}
	members, err := s.store.ListMembers(poolID)
	if err != nil {
		return err
	}
	priorities, snapshotsByRemote, err := s.poolSchedulingPrioritiesFor(pool, members)
	if err != nil {
		return err
	}
	_, target, adminAPIKey, err := s.loadAdminTarget()
	if err != nil {
		return err
	}
	client := s.adminFactory()
	adminTarget := sub2api.AdminTarget{BaseURL: target.BaseURL, APIKey: adminAPIKey}
	var reconcileErrors []error
	for i := range members {
		member := &members[i]
		if member.RemoteAccountID == nil || member.BindingStatus == "invalid" || member.BindingStatus == "orphaned" {
			continue
		}
		desiredPriority := priorities[member.ID]
		if desiredPriority <= 0 {
			desiredPriority = 1
		}
		desiredLoadFactor := automaticLoadFactor(member.Concurrency)
		if member.Weight != desiredLoadFactor || member.Priority != normalizeSchedulingPriority(member.Priority) {
			member.Weight = desiredLoadFactor
			member.Priority = normalizeSchedulingPriority(member.Priority)
			if updateErr := s.store.UpdateMember(member); updateErr != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("update member %d automatic scheduling fields: %w", member.ID, updateErr))
				continue
			}
		}
		snapshot := snapshotsByRemote[*member.RemoteAccountID]
		if snapshot != nil && snapshot.Priority == desiredPriority && snapshot.Concurrency == member.Concurrency && snapshot.Weight == desiredLoadFactor {
			continue
		}
		updated, updateErr := client.UpdateAccountScheduling(ctx, adminTarget, *member.RemoteAccountID, sub2api.AdminAccountSchedulingUpdate{
			Concurrency: member.Concurrency,
			Priority:    desiredPriority,
			LoadFactor:  desiredLoadFactor,
		})
		if updateErr != nil {
			if connector.HTTPStatusCode(updateErr) == http.StatusNotFound {
				if _, orphanErr := s.store.MarkMembersOrphaned([]int64{*member.RemoteAccountID}); orphanErr != nil {
					reconcileErrors = append(reconcileErrors, fmt.Errorf("mark member %d orphaned: %w", member.ID, orphanErr))
					continue
				}
				_ = s.appendAudit(&pool.ID, &member.ID, member.RemoteAccountID, "member_orphaned", source, true, snapshot, nil, nil,
					"remote account no longer exists; automatic ranking stopped", "")
				continue
			}
			reconcileErrors = append(reconcileErrors, fmt.Errorf("update member %d scheduling: %w", member.ID, redactSecretError(updateErr, adminAPIKey)))
			continue
		}
		if refreshed, refreshErr := client.GetAccount(ctx, adminTarget, *member.RemoteAccountID); refreshErr == nil {
			updated = refreshed
		}
		if updated != nil {
			s.saveRemoteSchedulingSnapshot(updated, *member.RemoteAccountID)
		}
		_ = s.appendAudit(&pool.ID, &member.ID, member.RemoteAccountID, "member_scheduling_rank", source, true, schedulingAuditState(snapshot), schedulingAuditState(updated), map[string]any{
			"automatic_priority": desiredPriority,
			"load_factor":        desiredLoadFactor,
		}, "automatic scheduling fields applied", "")
	}
	return errors.Join(reconcileErrors...)
}

func schedulingAuditState(account any) any {
	switch item := account.(type) {
	case *storage.MainStationAccountSnapshot:
		if item == nil {
			return nil
		}
		return map[string]any{
			"priority":    item.Priority,
			"concurrency": item.Concurrency,
			"load_factor": item.Weight,
			"schedulable": item.Schedulable,
			"status":      item.Status,
		}
	case *sub2api.AdminAccount:
		if item == nil {
			return nil
		}
		return map[string]any{
			"priority":    item.Priority,
			"concurrency": item.Concurrency,
			"load_factor": item.LoadFactor,
			"schedulable": item.Schedulable,
			"status":      item.Status,
		}
	default:
		return nil
	}
}

func validCostSnapshot(member *storage.MainAccountPoolMember, now time.Time) bool {
	return member != nil && member.LastCostMicros != nil && *member.LastCostMicros > 0 &&
		member.LastCostSource != "remote_account_estimate" &&
		(member.LastCostExpiresAt == nil || now.Before(*member.LastCostExpiresAt))
}
