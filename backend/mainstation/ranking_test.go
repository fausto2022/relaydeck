package mainstation

import (
	"context"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
)

func TestRankSchedulingSignalsUsesHealthPriorityCostAndStability(t *testing.T) {
	signals := []schedulingRankSignal{
		{MemberID: 1, HealthBand: 0, Priority: 1, CostKnown: true, CostMicros: 2_000_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 2, HealthBand: 0, Priority: 1, CostKnown: true, CostMicros: 1_000_000, SuccessBucket: 2, LatencyBucket: 2},
		{MemberID: 3, HealthBand: 0, Priority: 8, CostKnown: true, CostMicros: 500_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 4, HealthBand: 2, Priority: 1, CostKnown: true, CostMicros: 100_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 5, HealthBand: 0, Priority: 1, CostKnown: true, CostMicros: 1_000_000, SuccessBucket: 2, LatencyBucket: 2},
	}

	priorities := rankSchedulingSignals(signals, "asc")
	if priorities[3] != 10 || priorities[1] != 20 || priorities[2] != 30 || priorities[5] != 40 || priorities[4] != 50 {
		t.Fatalf("score-based sparse scheduling order = %#v", priorities)
	}
}

func TestRankSchedulingSignalsPrefersTaggedHealthyAccounts(t *testing.T) {
	signals := []schedulingRankSignal{
		{MemberID: 1, HealthBand: 0, Priority: 1, CostKnown: true, CostMicros: 100_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 2, HealthBand: 0, Preferred: true, Priority: 99, CostKnown: true, CostMicros: 9_000_000, SuccessBucket: 3, LatencyBucket: 3},
		{MemberID: 3, HealthBand: 3, Preferred: true, Priority: 1, CostKnown: true, CostMicros: 10_000, SuccessBucket: 0, LatencyBucket: 0},
	}

	priorities := rankSchedulingSignals(signals, "asc")
	if priorities[2] != 10 || priorities[1] != 20 || priorities[3] != 30 {
		t.Fatalf("preferred scheduling order = %#v", priorities)
	}
}

func TestRankSchedulingSignalsLatencyCanMoveHigherBasePriorityBehindAnotherAccount(t *testing.T) {
	priorities := rankSchedulingSignals([]schedulingRankSignal{
		{MemberID: 1, HealthBand: 0, Priority: 5, SuccessBucket: 0, LatencyBucket: 3},
		{MemberID: 2, HealthBand: 0, Priority: 10, SuccessBucket: 0, LatencyBucket: 0},
	}, "stability")
	if priorities[2] >= priorities[1] {
		t.Fatalf("low-latency account should rank first: %#v", priorities)
	}
}

func TestRankSchedulingSignalsPreservesIncreasingRemotePriorityAnchors(t *testing.T) {
	priorities := rankSchedulingSignals([]schedulingRankSignal{
		{MemberID: 1, Priority: 1, CurrentPriority: 10},
		{MemberID: 3, Priority: 10, CurrentPriority: 30},
		{MemberID: 2, Priority: 5, CurrentPriority: 20},
	}, "stability")
	if priorities[1] != 10 || priorities[2] != 20 || priorities[3] != 30 {
		t.Fatalf("sparse anchors should minimize updates while following order: %#v", priorities)
	}
}

func TestAutomaticSchedulingDefaults(t *testing.T) {
	if normalizeSchedulingPriority(0) != 1 || normalizeSchedulingPriority(-1) != 1 {
		t.Fatal("invalid priority must default to 1")
	}
	if normalizeSchedulingPriority(99) != 99 {
		t.Fatal("positive numeric priority must be preserved")
	}
	if automaticLoadFactor(37) != 37 || automaticLoadFactor(0) != 1 {
		t.Fatal("automatic load factor must follow concurrency")
	}
}

func TestRunDueRankingsHonorsPoolIntervalAndClearsDirtyState(t *testing.T) {
	service, db, admin, _ := newTestService(t)
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	configureTestStation(t, service)
	admin.groups = []sub2api.AdminGroup{{ID: 11, Name: "default", RateMultiplier: 1, Status: "active"}}
	admin.accounts = []sub2api.AdminAccount{{ID: 21, Name: "existing", Status: "active", Schedulable: true, Priority: 10}}
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	channel := createTestChannel(t, db)
	groups, err := service.ListGroups(false)
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %#v, err=%v", groups, err)
	}
	pool, err := service.CreatePool(PoolInput{
		Name: "pool", Platform: "openai", TargetGroupIDs: []uint{groups[0].ID}, RankingIntervalSeconds: 60,
	})
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	remoteID := int64(21)
	if _, err := service.CreateMember(context.Background(), pool.ID, MemberInput{
		OwnershipMode: "bound", SourceChannelID: channel.ID, RemoteAccountID: &remoteID,
		ManualBindingConfirmed: true, Enabled: boolPtr(true),
	}); err != nil {
		t.Fatalf("create member: %v", err)
	}

	service.RunDueRankings(context.Background())
	if len(admin.schedulingUpdates) != 1 {
		t.Fatalf("initial due ranking updates = %#v", admin.schedulingUpdates)
	}
	first, err := service.store.FindPool(pool.ID)
	if err != nil || first.RankingDirtyAt != nil || first.LastRankingAt == nil {
		t.Fatalf("initial ranking state = %#v, err=%v", first, err)
	}

	now = now.Add(time.Second)
	if err := service.markPoolRankingDirty(pool.ID); err != nil {
		t.Fatalf("mark ranking dirty: %v", err)
	}
	service.RunDueRankings(context.Background())
	notDue, err := service.store.FindPool(pool.ID)
	if err != nil || notDue.RankingDirtyAt == nil || !notDue.LastRankingAt.Equal(*first.LastRankingAt) {
		t.Fatalf("ranking should remain pending before interval: %#v, err=%v", notDue, err)
	}

	now = now.Add(59 * time.Second)
	service.RunDueRankings(context.Background())
	due, err := service.store.FindPool(pool.ID)
	if err != nil || due.RankingDirtyAt != nil || due.LastRankingAt == nil || !due.LastRankingAt.Equal(now) {
		t.Fatalf("due ranking state = %#v, err=%v", due, err)
	}
}

func TestPoolSchedulingPrioritiesMovesLockedAccountBehindSchedulableAccount(t *testing.T) {
	service, _, admin, pool, first := createBoundSchedulingMember(t)
	admin.accounts = append(admin.accounts, sub2api.AdminAccount{
		ID: 22, Name: "second", Platform: "openai", Status: "active", Schedulable: true, Priority: 20, GroupIDs: []int64{11},
	})
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync second account: %v", err)
	}
	remoteID := int64(22)
	second, err := service.CreateMember(context.Background(), pool.ID, MemberInput{
		OwnershipMode: "bound", SourceChannelID: first.SourceChannelID, RemoteAccountID: &remoteID,
		ManualBindingConfirmed: true, Enabled: boolPtr(true), Priority: 10,
	})
	if err != nil {
		t.Fatalf("create second member: %v", err)
	}
	if _, err := service.ActivateGuardLock(context.Background(), *first.RemoteAccountID, "manual", "pause", nil, "admin"); err != nil {
		t.Fatalf("pause first member: %v", err)
	}

	priorities, err := service.poolSchedulingPriorities(pool.ID)
	if err != nil {
		t.Fatalf("calculate priorities: %v", err)
	}
	if priorities[first.ID] <= priorities[second.ID] {
		t.Fatalf("locked member should rank last: %#v", priorities)
	}
}
