package mainstation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/storage"
	"gorm.io/gorm"
)

func TestCleanupDisabledManagedMemberDeletesExactRemoteResourcesAfterDeadline(t *testing.T) {
	service, db, admin, channels := newTestService(t)
	configureTestStation(t, service)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	channel := createTestChannel(t, db)
	pool := &storage.MainAccountPool{Name: "cleanup-pool", Platform: "openai", Enabled: true, DisabledCleanupSeconds: 3600}
	if err := service.store.CreatePool(pool, nil); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	remoteID, keyID := int64(21), int64(77)
	disabledSince := now.Add(-time.Hour)
	member := &storage.MainAccountPoolMember{
		PoolID: pool.ID, AccountName: "managed", OwnershipMode: "managed", BindingStatus: "verified", Status: "active",
		SourceChannelID: channel.ID, SourceAPIKeyID: &keyID, SourceAPIKeyName: "group-K7P2", SourceAPIKeyManaged: true,
		RemoteAccountID: &remoteID, RemoteAccountName: "managed", Enabled: false, DisabledSince: &disabledSince,
	}
	if err := service.store.CreateMember(member); err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := db.Model(member).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable member: %v", err)
	}
	admin.accounts = []sub2api.AdminAccount{{ID: remoteID, Name: "managed", Status: "active", Schedulable: false}}

	service.CleanupDisabledManagedMembers(context.Background())

	if len(admin.deletedAccounts) != 1 || admin.deletedAccounts[0] != remoteID || len(channels.deletedKeys) != 1 || channels.deletedKeys[0] != keyID {
		t.Fatalf("deleted resources: accounts=%v keys=%v", admin.deletedAccounts, channels.deletedKeys)
	}
	if _, err := service.store.FindMember(pool.ID, member.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("member was not deleted: %v", err)
	}
}

func TestCleanupDisabledManagedMembersPreservesUnsafeOrYoungMembers(t *testing.T) {
	service, db, admin, channels := newTestService(t)
	configureTestStation(t, service)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	channel := createTestChannel(t, db)
	pool := &storage.MainAccountPool{Name: "cleanup-pool", Platform: "openai", Enabled: true, DisabledCleanupSeconds: 3600}
	if err := service.store.CreatePool(pool, nil); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	for i, tc := range []struct {
		managed bool
		since   time.Time
	}{
		{managed: true, since: now.Add(-30 * time.Minute)},
		{managed: false, since: now.Add(-2 * time.Hour)},
	} {
		remoteID, keyID := int64(30+i), int64(80+i)
		member := &storage.MainAccountPoolMember{
			PoolID: pool.ID, AccountName: "preserved", OwnershipMode: "managed", BindingStatus: "verified", Status: "active",
			SourceChannelID: channel.ID, SourceAPIKeyID: &keyID, SourceAPIKeyManaged: tc.managed,
			RemoteAccountID: &remoteID, RemoteAccountName: "preserved", Enabled: true, DisabledSince: &tc.since,
		}
		if err := service.store.CreateMember(member); err != nil {
			t.Fatalf("create member %d: %v", i, err)
		}
		admin.accounts = append(admin.accounts, sub2api.AdminAccount{ID: remoteID, Status: "active", Schedulable: false})
	}

	service.CleanupDisabledManagedMembers(context.Background())

	if len(admin.deletedAccounts) != 0 || len(channels.deletedKeys) != 0 {
		t.Fatalf("unsafe resources were deleted: accounts=%v keys=%v", admin.deletedAccounts, channels.deletedKeys)
	}
}

func TestCleanupDisabledMembersDeletesPreviewedAccountAndExactSourceKey(t *testing.T) {
	service, db, admin, channels := newTestService(t)
	configureTestStation(t, service)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	channel := createTestChannel(t, db)
	pool := &storage.MainAccountPool{Name: "manual-cleanup-pool", Platform: "openai", Enabled: true}
	if err := service.store.CreatePool(pool, nil); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	remoteID, keyID := int64(121), int64(177)
	disabledSince := now.Add(-20 * time.Minute)
	member := &storage.MainAccountPoolMember{
		PoolID: pool.ID, AccountName: "disabled-account", OwnershipMode: "bound", BindingStatus: "manual_confirmed", Status: "disabled",
		SourceChannelID: channel.ID, SourceAPIKeyID: &keyID, SourceAPIKeyName: "manual-key",
		RemoteAccountID: &remoteID, RemoteAccountName: "disabled-account", Enabled: true, DisabledSince: &disabledSince,
	}
	if err := service.store.CreateMember(member); err != nil {
		t.Fatalf("create member: %v", err)
	}
	admin.accounts = []sub2api.AdminAccount{{ID: remoteID, Name: member.AccountName, Status: "active", Schedulable: false}}

	preview, err := service.PreviewDisabledMemberCleanup(pool.ID)
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if preview.Eligible != 1 || preview.Skipped != 0 || len(preview.Candidates) != 1 || preview.Candidates[0].SourceAPIKeyID != keyID {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	result, err := service.CleanupDisabledMembers(context.Background(), pool.ID, DisabledMemberCleanupInput{Confirm: true})
	if err != nil {
		t.Fatalf("cleanup disabled members: %v", err)
	}
	if result.Attempted != 1 || result.Deleted != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected cleanup result: %#v", result)
	}
	if len(admin.deletedAccounts) != 1 || admin.deletedAccounts[0] != remoteID || len(channels.deletedKeys) != 1 || channels.deletedKeys[0] != keyID {
		t.Fatalf("deleted resources: accounts=%v keys=%v", admin.deletedAccounts, channels.deletedKeys)
	}
	if _, err := service.store.FindMember(pool.ID, member.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("member was not deleted: %v", err)
	}
	var audit storage.MainAccountAuditLog
	if err := db.Where("action = ?", "member_disabled_cleanup_manual").First(&audit).Error; err != nil {
		t.Fatalf("load cleanup audit: %v", err)
	}
}

func TestCleanupDisabledMembersSkipsAccountRecoveredAfterPreview(t *testing.T) {
	service, db, admin, channels := newTestService(t)
	configureTestStation(t, service)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	channel := createTestChannel(t, db)
	pool := &storage.MainAccountPool{Name: "manual-cleanup-recovered", Platform: "openai", Enabled: true}
	if err := service.store.CreatePool(pool, nil); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	remoteID, keyID := int64(221), int64(277)
	disabledSince := now.Add(-time.Hour)
	member := &storage.MainAccountPoolMember{
		PoolID: pool.ID, AccountName: "recovered-account", OwnershipMode: "managed", BindingStatus: "verified", Status: "active",
		SourceChannelID: channel.ID, SourceAPIKeyID: &keyID, SourceAPIKeyManaged: true,
		RemoteAccountID: &remoteID, RemoteAccountName: "recovered-account", Enabled: true, DisabledSince: &disabledSince,
	}
	if err := service.store.CreateMember(member); err != nil {
		t.Fatalf("create member: %v", err)
	}
	admin.accounts = []sub2api.AdminAccount{{ID: remoteID, Name: member.AccountName, Status: "active", Schedulable: true}}

	result, err := service.CleanupDisabledMembers(context.Background(), pool.ID, DisabledMemberCleanupInput{Confirm: true})
	if err != nil {
		t.Fatalf("cleanup disabled members: %v", err)
	}
	if result.Attempted != 1 || result.Deleted != 0 || result.Skipped != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected cleanup result: %#v", result)
	}
	if len(admin.deletedAccounts) != 0 || len(channels.deletedKeys) != 0 {
		t.Fatalf("recovered resources were deleted: accounts=%v keys=%v", admin.deletedAccounts, channels.deletedKeys)
	}
	stored, err := service.store.FindMember(pool.ID, member.ID)
	if err != nil || stored.DisabledSince != nil {
		t.Fatalf("recovered member state = %#v, err=%v", stored, err)
	}
}

func TestPreviewDisabledMemberCleanupSkipsSharedSourceKey(t *testing.T) {
	service, db, _, _ := newTestService(t)
	configureTestStation(t, service)
	channel := createTestChannel(t, db)
	pool := &storage.MainAccountPool{Name: "shared-key-cleanup", Platform: "openai", Enabled: true}
	if err := service.store.CreatePool(pool, nil); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	keyID := int64(377)
	disabledSince := time.Now().Add(-time.Hour)
	for i := 0; i < 2; i++ {
		remoteID := int64(320 + i)
		member := &storage.MainAccountPoolMember{
			PoolID: pool.ID, AccountName: fmt.Sprintf("shared-%d", i), OwnershipMode: "bound", BindingStatus: "manual_confirmed", Status: "disabled",
			SourceChannelID: channel.ID, SourceAPIKeyID: &keyID, RemoteAccountID: &remoteID,
		}
		if i == 0 {
			member.DisabledSince = &disabledSince
		}
		if err := service.store.CreateMember(member); err != nil {
			t.Fatalf("create member %d: %v", i, err)
		}
	}

	preview, err := service.PreviewDisabledMemberCleanup(pool.ID)
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if preview.Eligible != 0 || preview.Skipped != 2 {
		t.Fatalf("shared key should not be eligible: %#v", preview)
	}
}

func TestReconcileAccountTracksContinuousDisabledTimeWithoutPoolShutdown(t *testing.T) {
	service, _, admin, pool, member := createBoundSchedulingMember(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	member.Enabled = false
	if err := service.store.UpdateMember(member); err != nil {
		t.Fatalf("disable member: %v", err)
	}
	if _, err := service.ReconcileAccount(context.Background(), *member.RemoteAccountID, "manual"); err != nil {
		t.Fatalf("reconcile disabled member: %v", err)
	}
	stored, err := service.store.FindMember(pool.ID, member.ID)
	if err != nil || stored.DisabledSince == nil || !stored.DisabledSince.Equal(now) {
		t.Fatalf("disabled since = %#v, err=%v", stored, err)
	}
	stored.Enabled = true
	if err := service.store.UpdateMember(stored); err != nil {
		t.Fatalf("enable member: %v", err)
	}
	if _, err := service.ReconcileAccount(context.Background(), *member.RemoteAccountID, "manual"); err != nil {
		t.Fatalf("reconcile recovered member: %v", err)
	}
	stored, err = service.store.FindMember(pool.ID, member.ID)
	if err != nil || stored.DisabledSince != nil || !admin.accounts[0].Schedulable {
		t.Fatalf("recovered member = %#v, remote=%#v, err=%v", stored, admin.accounts[0], err)
	}
	pool.Enabled = false
	if err := service.store.UpdatePool(&pool.MainAccountPool, pool.TargetGroupIDs); err != nil {
		t.Fatalf("disable pool: %v", err)
	}
	if _, err := service.ReconcileAccount(context.Background(), *member.RemoteAccountID, "manual"); err != nil {
		t.Fatalf("reconcile disabled pool: %v", err)
	}
	stored, err = service.store.FindMember(pool.ID, member.ID)
	if err != nil || stored.DisabledSince != nil {
		t.Fatalf("pool shutdown started cleanup timer: %#v, err=%v", stored, err)
	}
}
