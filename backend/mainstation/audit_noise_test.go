package mainstation

import (
	"context"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestStableSchedulingReconcileDoesNotWriteAudit(t *testing.T) {
	service, db, _, _, member := createBoundSchedulingMember(t)
	var before int64
	if err := db.Model(&storage.MainAccountAuditLog{}).
		Where("action = ?", "schedulable_reconcile").
		Count(&before).Error; err != nil {
		t.Fatalf("count scheduling audits: %v", err)
	}
	if _, err := service.ReconcileAccount(context.Background(), *member.RemoteAccountID, "scheduler"); err != nil {
		t.Fatalf("reconcile stable scheduling: %v", err)
	}
	var after int64
	if err := db.Model(&storage.MainAccountAuditLog{}).
		Where("action = ?", "schedulable_reconcile").
		Count(&after).Error; err != nil {
		t.Fatalf("count stable scheduling audits: %v", err)
	}
	if after != before {
		t.Fatalf("stable scheduling audits = %d -> %d", before, after)
	}
}

func TestSchedulerProfitEvaluationDoesNotWriteAudit(t *testing.T) {
	service, db, admin, _ := newTestService(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, shanghaiLocation())
	service.now = func() time.Time { return now }
	pool, _, _ := createProfitMember(
		t,
		service,
		db,
		admin,
		now,
		0.8,
		`{"mode":"observe","minimum_margin_basis_points":0,"risk_confirmations":2,"cost_max_age_minutes":60}`,
	)
	if _, err := service.EvaluatePool(context.Background(), pool.ID, "scheduler"); err != nil {
		t.Fatalf("scheduler profit evaluation: %v", err)
	}
	var schedulerAudits int64
	if err := db.Model(&storage.MainAccountAuditLog{}).
		Where("action = ?", "pool_profit_evaluate").
		Count(&schedulerAudits).Error; err != nil {
		t.Fatalf("count scheduler profit audits: %v", err)
	}
	if schedulerAudits != 0 {
		t.Fatalf("scheduler profit audits = %d", schedulerAudits)
	}
	if _, err := service.EvaluatePool(context.Background(), pool.ID, "manual"); err != nil {
		t.Fatalf("manual profit evaluation: %v", err)
	}
	var manualAudits int64
	if err := db.Model(&storage.MainAccountAuditLog{}).
		Where("action = ?", "pool_profit_evaluate").
		Count(&manualAudits).Error; err != nil {
		t.Fatalf("count manual profit audits: %v", err)
	}
	if manualAudits != 1 {
		t.Fatalf("manual profit audits = %d", manualAudits)
	}
}

func TestShouldAuditSuccessfulSync(t *testing.T) {
	if !shouldAuditSuccessfulSync("manual", &SyncResult{}) {
		t.Fatal("manual sync should be audited")
	}
	if shouldAuditSuccessfulSync("scheduler", &SyncResult{}) {
		t.Fatal("unchanged scheduler sync should not be audited")
	}
	if !shouldAuditSuccessfulSync("scheduler", &SyncResult{PricingChanged: true}) {
		t.Fatal("pricing change should be audited")
	}
	if shouldAuditSuccessfulSync("scheduler", &SyncResult{SourceBindingsMissing: 1}) {
		t.Fatal("persistent missing binding should not be audited")
	}
	if !shouldAuditSuccessfulSync("scheduler", &SyncResult{SourceBindingsUpdated: 1}) {
		t.Fatal("binding change should be audited")
	}
}
