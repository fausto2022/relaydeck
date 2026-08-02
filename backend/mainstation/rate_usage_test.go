package mainstation

import (
	"context"
	"testing"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestGetRateUsageIncludesMainStationAccounts(t *testing.T) {
	service, db, admin, _ := newTestService(t)
	configureTestStation(t, service)
	admin.groups = []sub2api.AdminGroup{{
		ID: 11, Name: "main-openai", Platform: "openai", RateMultiplier: 1, Status: "active",
	}}
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	groups, err := service.ListGroups(false)
	if err != nil || len(groups) != 1 {
		t.Fatalf("list groups: groups=%#v err=%v", groups, err)
	}
	poolID, err := service.GroupPoolID(groups[0].ID)
	if err != nil {
		t.Fatalf("resolve pool: %v", err)
	}
	channel := createTestChannel(t, db)
	sourceGroupID := int64(301)
	rate := &storage.RateSnapshot{
		ChannelID: channel.ID, RemoteGroupID: &sourceGroupID, ModelName: "source-openai", Platform: "openai", Ratio: 0.1,
	}
	if err := db.Create(rate).Error; err != nil {
		t.Fatalf("create rate: %v", err)
	}
	keyID := int64(71)
	pending := &storage.MainAccountPoolMember{
		PoolID: poolID, SourceChannelID: channel.ID, SourceGroupID: &sourceGroupID,
		SourceGroupName: "source-openai", SourceAPIKeyID: &keyID, SourceAPIKeyName: "source-openai-A1B2",
		SourceAPIKeyManaged: true, AccountName: "source-openai", OwnershipMode: "managed",
		BindingStatus: "pending", Status: "pending", Enabled: true, LastHealthStatus: "unknown",
	}
	if err := service.store.CreateMember(pending); err != nil {
		t.Fatalf("create pending member: %v", err)
	}

	usage, err := service.GetRateUsage(channel.ID, rate.ID)
	if err != nil {
		t.Fatalf("get initializing usage: %v", err)
	}
	if !usage.Connected || usage.Status != "initializing" || usage.AccountCount != 1 || len(usage.Groups) != 1 {
		t.Fatalf("initializing usage = %#v", usage)
	}
	if !usage.Groups[0].Connected {
		t.Fatalf("initializing group should be connected: %#v", usage.Groups[0])
	}
	account := usage.Groups[0].Accounts[0]
	if account.MemberID != pending.ID || account.SourceAPIKeyName != "source-openai-A1B2" || !account.SourceAPIKeyManaged {
		t.Fatalf("usage account = %#v", account)
	}

	remoteAccountID := int64(21)
	pending.RemoteAccountID = &remoteAccountID
	pending.RemoteAccountName = "main-source-openai"
	pending.BindingStatus = "verified"
	pending.Status = "active"
	pending.LastHealthStatus = "success"
	if err := service.store.UpdateMember(pending); err != nil {
		t.Fatalf("activate member: %v", err)
	}
	usage, err = service.GetRateUsage(channel.ID, rate.ID)
	if err != nil {
		t.Fatalf("get connected usage: %v", err)
	}
	if !usage.Connected || usage.Status != "connected" || usage.Groups[0].Accounts[0].MainAccountName != "main-source-openai" {
		t.Fatalf("connected usage = %#v", usage)
	}
}

func TestGetRateUsageReportsAbnormalBinding(t *testing.T) {
	service, db, admin, _ := newTestService(t)
	configureTestStation(t, service)
	admin.groups = []sub2api.AdminGroup{{
		ID: 11, Name: "main-openai", Platform: "openai", RateMultiplier: 1, Status: "active",
	}}
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	groups, err := service.ListGroups(false)
	if err != nil || len(groups) != 1 {
		t.Fatalf("list groups: groups=%#v err=%v", groups, err)
	}
	poolID, err := service.GroupPoolID(groups[0].ID)
	if err != nil {
		t.Fatalf("resolve pool: %v", err)
	}
	channel := createTestChannel(t, db)
	rate := &storage.RateSnapshot{ChannelID: channel.ID, ModelName: "source-openai", Platform: "openai", Ratio: 0.1}
	if err := db.Create(rate).Error; err != nil {
		t.Fatalf("create rate: %v", err)
	}
	member := &storage.MainAccountPoolMember{
		PoolID: poolID, SourceChannelID: channel.ID, SourceGroupName: rate.ModelName,
		OwnershipMode: "managed", BindingStatus: "orphaned", Status: "orphaned", Enabled: false,
	}
	if err := service.store.CreateMember(member); err != nil {
		t.Fatalf("create orphaned member: %v", err)
	}

	usage, err := service.GetRateUsage(channel.ID, rate.ID)
	if err != nil {
		t.Fatalf("get abnormal usage: %v", err)
	}
	if usage.Connected || usage.Status != "abnormal" || usage.AccountCount != 1 {
		t.Fatalf("abnormal usage = %#v", usage)
	}
}
