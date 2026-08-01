package mainstation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector/sub2api"
	"github.com/fausto2022/relaydeck/backend/storage"
)

const disabledManagedCleanupInterval = time.Minute

// CleanupDisabledManagedMembers removes only RelayDeck-managed resources whose
// exact main-station account and source API key IDs are both still recorded.
func (s *Service) CleanupDisabledManagedMembers(ctx context.Context) {
	if !s.disabledCleanupMu.TryLock() {
		return
	}
	defer s.disabledCleanupMu.Unlock()
	now := s.now()
	if !s.disabledCleanupAt.IsZero() && now.Sub(s.disabledCleanupAt) < disabledManagedCleanupInterval {
		return
	}
	s.disabledCleanupAt = now
	members, err := s.store.ListDisabledManagedMembersForCleanup()
	if err != nil {
		if s.log != nil {
			s.log.Warn("list disabled managed members for cleanup", "err", err)
		}
		return
	}
	for i := range members {
		if ctx.Err() != nil {
			return
		}
		if err := s.cleanupDisabledManagedMember(ctx, &members[i], now); err != nil && s.log != nil {
			s.log.Warn("cleanup disabled managed member", "err", err, "pool_id", members[i].PoolID, "member_id", members[i].ID)
		}
	}
}

func (s *Service) cleanupDisabledManagedMember(ctx context.Context, member *storage.MainAccountPoolMember, now time.Time) error {
	if member == nil || member.DisabledSince == nil || member.RemoteAccountID == nil || member.SourceAPIKeyID == nil ||
		member.OwnershipMode != "managed" || !member.SourceAPIKeyManaged {
		return nil
	}
	pool, err := s.store.FindPool(member.PoolID)
	if err != nil {
		return err
	}
	if pool.DisabledCleanupSeconds <= 0 || now.Before(member.DisabledSince.Add(time.Duration(pool.DisabledCleanupSeconds)*time.Second)) {
		return nil
	}
	config, target, adminAPIKey, err := s.loadAdminTarget()
	if err != nil {
		return err
	}
	if !config.Enabled || !pool.Enabled {
		return nil
	}
	client := s.adminFactory()
	adminTarget := sub2api.AdminTarget{BaseURL: target.BaseURL, APIKey: adminAPIKey}
	remote, err := client.GetAccount(ctx, adminTarget, *member.RemoteAccountID)
	if err != nil && !missingRemoteResource(err) {
		return fmt.Errorf("verify disabled main station account: %w", redactSecretError(err, adminAPIKey))
	}
	if remote != nil {
		locks, lockErr := s.store.ListActiveGuardLocks(*member.RemoteAccountID)
		if lockErr != nil {
			return fmt.Errorf("verify disabled main station account locks: %w", lockErr)
		}
		bindingValid := member.BindingStatus == "verified" || member.BindingStatus == "manual_confirmed"
		shouldSchedule := member.Enabled && strings.EqualFold(remote.Status, "active") && bindingValid && len(locks) == 0
		if remote.Schedulable || shouldSchedule {
			if clearErr := s.store.UpdateMemberDisabledSince(member.ID, nil); clearErr != nil {
				return clearErr
			}
			if shouldSchedule && !remote.Schedulable {
				if dirtyErr := s.store.MarkMemberSchedulingDirty(member.ID, now); dirtyErr != nil {
					return dirtyErr
				}
			}
			return nil
		}
	}
	before := *member
	if remote != nil {
		if err := client.DeleteAccount(ctx, adminTarget, *member.RemoteAccountID); err != nil && !missingRemoteResource(err) {
			return fmt.Errorf("delete disabled main station account: %w", redactSecretError(err, adminAPIKey))
		}
	}
	if err := s.channelSvc.DeleteAPIKey(ctx, member.SourceChannelID, *member.SourceAPIKeyID); err != nil && !missingRemoteResource(err) {
		return fmt.Errorf("delete disabled managed source api key: %w", err)
	}
	if err := s.store.DeleteMember(member.PoolID, member.ID); err != nil {
		return err
	}
	if err := s.store.MarkAccountSnapshotMissing(*member.RemoteAccountID, now); err != nil && s.log != nil {
		s.log.Warn("mark auto-cleaned account snapshot missing", "err", err, "remote_account_id", *member.RemoteAccountID)
	}
	if err := s.markPoolRankingDirty(member.PoolID); err != nil && s.log != nil {
		s.log.Warn("mark pool ranking dirty after disabled cleanup", "err", err, "pool_id", member.PoolID)
	}
	detail := fmt.Sprintf("账号持续停用 %s，已删除主站托管账号和上游托管 Key", formatCleanupDuration(time.Duration(pool.DisabledCleanupSeconds)*time.Second))
	_ = s.appendAudit(&member.PoolID, &member.ID, member.RemoteAccountID, "member_disabled_cleanup", "scheduler", true,
		before, nil, map[string]any{"source_channel_id": member.SourceChannelID, "source_api_key_id": *member.SourceAPIKeyID}, detail, "")
	return nil
}

func formatCleanupDuration(value time.Duration) string {
	if value%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d 天", int(value/(24*time.Hour)))
	}
	return fmt.Sprintf("%d 小时", int(value/time.Hour))
}
