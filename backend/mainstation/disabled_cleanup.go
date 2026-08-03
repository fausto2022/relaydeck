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

// PreviewDisabledMemberCleanup lists disabled members that have the exact
// remote account and source API key identifiers required for a safe cleanup.
func (s *Service) PreviewDisabledMemberCleanup(poolID uint) (*DisabledMemberCleanupPreview, error) {
	if _, err := s.store.FindPool(poolID); err != nil {
		return nil, err
	}
	members, err := s.store.ListMembers(poolID)
	if err != nil {
		return nil, err
	}
	result := &DisabledMemberCleanupPreview{PoolID: poolID, Candidates: make([]DisabledMemberCleanupCandidate, 0)}
	for i := range members {
		member := &members[i]
		if member.DisabledSince == nil || member.RemoteAccountID == nil || member.SourceAPIKeyID == nil ||
			*member.RemoteAccountID <= 0 || member.SourceChannelID == 0 || *member.SourceAPIKeyID <= 0 {
			result.Skipped++
			continue
		}
		otherReferences, countErr := s.store.CountOtherMembersUsingSourceAPIKey(member.SourceChannelID, *member.SourceAPIKeyID, member.ID)
		if countErr != nil {
			return nil, countErr
		}
		if otherReferences > 0 {
			result.Skipped++
			continue
		}
		result.Candidates = append(result.Candidates, DisabledMemberCleanupCandidate{
			MemberID: member.ID, AccountName: member.AccountName, RemoteAccountID: *member.RemoteAccountID,
			SourceChannelID: member.SourceChannelID, SourceAPIKeyID: *member.SourceAPIKeyID,
			SourceAPIKeyName: member.SourceAPIKeyName, DisabledSince: *member.DisabledSince,
		})
	}
	result.Eligible = len(result.Candidates)
	return result, nil
}

// CleanupDisabledMembers deletes only members returned by the cleanup preview.
// Each member is reloaded and its remote scheduling state is checked immediately
// before deleting the exact main-station account and source API key.
func (s *Service) CleanupDisabledMembers(ctx context.Context, poolID uint, in DisabledMemberCleanupInput) (*DisabledMemberCleanupResult, error) {
	if !in.Confirm {
		return nil, fmt.Errorf("disabled member cleanup requires explicit confirmation")
	}
	preview, err := s.PreviewDisabledMemberCleanup(poolID)
	if err != nil {
		return nil, err
	}
	result := &DisabledMemberCleanupResult{PoolID: poolID, Skipped: preview.Skipped, Errors: make([]string, 0)}
	if len(preview.Candidates) == 0 {
		return result, nil
	}
	_, target, adminAPIKey, err := s.loadAdminTarget()
	if err != nil {
		return nil, err
	}
	client := s.adminFactory()
	adminTarget := sub2api.AdminTarget{BaseURL: target.BaseURL, APIKey: adminAPIKey}
	now := s.now()
	for i := range preview.Candidates {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		candidate := &preview.Candidates[i]
		result.Attempted++
		member, findErr := s.store.FindMember(poolID, candidate.MemberID)
		if findErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：重新读取失败：%s", candidate.AccountName, sanitizeText(findErr.Error())))
			continue
		}
		if member.DisabledSince == nil || member.RemoteAccountID == nil || member.SourceAPIKeyID == nil ||
			*member.RemoteAccountID != candidate.RemoteAccountID || member.SourceChannelID != candidate.SourceChannelID ||
			*member.SourceAPIKeyID != candidate.SourceAPIKeyID {
			result.Skipped++
			continue
		}
		otherReferences, countErr := s.store.CountOtherMembersUsingSourceAPIKey(candidate.SourceChannelID, candidate.SourceAPIKeyID, member.ID)
		if countErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：复核上游 Key 使用状态失败：%s", candidate.AccountName, sanitizeText(countErr.Error())))
			continue
		}
		if otherReferences > 0 {
			result.Skipped++
			continue
		}
		remote, getErr := client.GetAccount(ctx, adminTarget, candidate.RemoteAccountID)
		if getErr != nil && !missingRemoteResource(getErr) {
			result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：复核主站状态失败：%s", candidate.AccountName, sanitizeText(redactSecretError(getErr, adminAPIKey).Error())))
			continue
		}
		if remote != nil && remote.Schedulable {
			if updateErr := s.store.UpdateMemberDisabledSince(member.ID, nil); updateErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：更新恢复状态失败：%s", candidate.AccountName, sanitizeText(updateErr.Error())))
				continue
			}
			result.Skipped++
			continue
		}
		before := *member
		if remote != nil {
			if deleteErr := client.DeleteAccount(ctx, adminTarget, candidate.RemoteAccountID); deleteErr != nil && !missingRemoteResource(deleteErr) {
				result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：删除主站账号失败：%s", candidate.AccountName, sanitizeText(redactSecretError(deleteErr, adminAPIKey).Error())))
				continue
			}
		}
		if deleteErr := s.channelSvc.DeleteAPIKey(ctx, candidate.SourceChannelID, candidate.SourceAPIKeyID); deleteErr != nil && !missingRemoteResource(deleteErr) {
			result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：删除上游 Key 失败：%s", candidate.AccountName, sanitizeText(deleteErr.Error())))
			continue
		}
		if deleteErr := s.store.DeleteMember(poolID, member.ID); deleteErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("账号 %s：删除本地记录失败：%s", candidate.AccountName, sanitizeText(deleteErr.Error())))
			continue
		}
		if snapshotErr := s.store.MarkAccountSnapshotMissing(candidate.RemoteAccountID, now); snapshotErr != nil && s.log != nil {
			s.log.Warn("mark manually cleaned account snapshot missing", "err", snapshotErr, "remote_account_id", candidate.RemoteAccountID)
		}
		result.Deleted++
		_ = s.appendAudit(&poolID, &member.ID, member.RemoteAccountID, "member_disabled_cleanup_manual", "manual", true,
			before, nil, map[string]any{"source_channel_id": candidate.SourceChannelID, "source_api_key_id": candidate.SourceAPIKeyID},
			"已一键清理停用主站账号和其精确绑定的上游 Key", "")
	}
	if result.Deleted > 0 {
		if dirtyErr := s.markPoolRankingDirty(poolID); dirtyErr != nil && s.log != nil {
			s.log.Warn("mark pool ranking dirty after manual disabled cleanup", "pool_id", poolID, "err", dirtyErr)
		}
	}
	return result, nil
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
