package mainstation

import (
	"sort"
	"strings"

	"github.com/fausto2022/relaydeck/backend/storage"
)

func (s *Service) GetRateUsage(channelID, rateID uint) (*RateUsage, error) {
	rate, err := s.rates.FindByID(channelID, rateID)
	if err != nil {
		return nil, err
	}
	members, err := s.store.ListAllMembers()
	if err != nil {
		return nil, err
	}
	channelRates, err := s.rates.ListByChannel(channelID)
	if err != nil {
		return nil, err
	}

	result := &RateUsage{
		ChannelID: channelID,
		RateID:    rateID,
		Status:    "unused",
		Groups:    []RateUsageGroup{},
	}
	groups := make(map[uint]*RateUsageGroup)
	hasInitializing := false
	hasAbnormal := false
	for i := range members {
		member := &members[i]
		if member.SourceChannelID != channelID || !sourceMemberMatchesRate(member, rate, len(channelRates)) {
			continue
		}
		groupIDs, listErr := s.store.ListPoolGroupIDs(member.PoolID)
		if listErr != nil {
			return nil, listErr
		}
		account := rateUsageAccount(member)
		usable := usableRateUsageMember(member)
		for _, groupID := range groupIDs {
			usageGroup := groups[groupID]
			if usageGroup == nil {
				group, findErr := s.targetGroups.FindByID(groupID)
				if findErr != nil {
					return nil, findErr
				}
				usageGroup = &RateUsageGroup{
					GroupID:   group.ID,
					GroupName: group.Name,
					Missing:   group.Missing,
					Accounts:  []RateUsageAccount{},
				}
				groups[groupID] = usageGroup
			}
			usageGroup.Accounts = append(usageGroup.Accounts, account)
			usageGroup.Connected = usageGroup.Connected || usable
		}
		result.AccountCount++
		if usable {
			result.Connected = true
			if member.RemoteAccountID == nil || strings.EqualFold(member.Status, "pending") {
				hasInitializing = true
			}
		} else {
			hasAbnormal = true
		}
	}

	groupIDs := make([]uint, 0, len(groups))
	for groupID := range groups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	for _, groupID := range groupIDs {
		usageGroup := groups[groupID]
		sort.Slice(usageGroup.Accounts, func(i, j int) bool {
			return usageGroup.Accounts[i].MemberID < usageGroup.Accounts[j].MemberID
		})
		result.Groups = append(result.Groups, *usageGroup)
	}

	switch {
	case result.Connected && hasInitializing:
		result.Status = "initializing"
	case result.Connected:
		result.Status = "connected"
	case hasAbnormal:
		result.Status = "abnormal"
	}
	return result, nil
}

func rateUsageAccount(member *storage.MainAccountPoolMember) RateUsageAccount {
	name := strings.TrimSpace(member.RemoteAccountName)
	if name == "" {
		name = strings.TrimSpace(member.AccountName)
	}
	return RateUsageAccount{
		MemberID:            member.ID,
		PoolID:              member.PoolID,
		MainAccountID:       member.RemoteAccountID,
		MainAccountName:     name,
		SourceAPIKeyID:      member.SourceAPIKeyID,
		SourceAPIKeyName:    member.SourceAPIKeyName,
		SourceAPIKeyManaged: member.SourceAPIKeyManaged,
		OwnershipMode:       member.OwnershipMode,
		BindingStatus:       member.BindingStatus,
		Status:              member.Status,
		Enabled:             member.Enabled,
		LastHealthStatus:    member.LastHealthStatus,
	}
}

func usableRateUsageMember(member *storage.MainAccountPoolMember) bool {
	return !strings.EqualFold(member.BindingStatus, "invalid") &&
		!strings.EqualFold(member.BindingStatus, "orphaned") &&
		!strings.EqualFold(member.Status, "orphaned")
}
