package mainstation

import "testing"

func TestRankSchedulingSignalsUsesHealthRoleCostAndStability(t *testing.T) {
	signals := []schedulingRankSignal{
		{MemberID: 1, HealthBand: 0, Role: primarySchedulingRole, CostKnown: true, CostMicros: 2_000_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 2, HealthBand: 0, Role: primarySchedulingRole, CostKnown: true, CostMicros: 1_000_000, SuccessBucket: 2, LatencyBucket: 2},
		{MemberID: 3, HealthBand: 0, Role: backupSchedulingRole, CostKnown: true, CostMicros: 500_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 4, HealthBand: 2, Role: primarySchedulingRole, CostKnown: true, CostMicros: 100_000, SuccessBucket: 0, LatencyBucket: 0},
		{MemberID: 5, HealthBand: 0, Role: primarySchedulingRole, CostKnown: true, CostMicros: 1_000_000, SuccessBucket: 2, LatencyBucket: 2},
	}

	priorities := rankSchedulingSignals(signals)
	if priorities[2] != 1 || priorities[5] != 1 {
		t.Fatalf("same healthy primary signals should share first priority: %#v", priorities)
	}
	if priorities[1] != 2 {
		t.Fatalf("higher-cost primary priority = %d, want 2", priorities[1])
	}
	if priorities[3] != 3 {
		t.Fatalf("healthy backup priority = %d, want 3", priorities[3])
	}
	if priorities[4] != 4 {
		t.Fatalf("degraded primary priority = %d, want 4", priorities[4])
	}
}

func TestAutomaticSchedulingDefaults(t *testing.T) {
	if normalizeSchedulingRole(0) != primarySchedulingRole || normalizeSchedulingRole(1) != primarySchedulingRole {
		t.Fatal("primary role normalization failed")
	}
	if normalizeSchedulingRole(backupSchedulingRole) != backupSchedulingRole {
		t.Fatal("backup role normalization failed")
	}
	if normalizeSchedulingRole(99) != primarySchedulingRole {
		t.Fatal("legacy numeric priority must default to primary")
	}
	if automaticLoadFactor(37) != 37 || automaticLoadFactor(0) != 1 {
		t.Fatal("automatic load factor must follow concurrency")
	}
}
