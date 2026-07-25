package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fausto2022/relaydeck/backend/connector"
	"github.com/fausto2022/relaydeck/backend/mainstation"
	"github.com/fausto2022/relaydeck/backend/rateranking"
	"github.com/fausto2022/relaydeck/backend/storage"
	"github.com/gin-gonic/gin"
)

type rateChangeOutput struct {
	ID                     uint      `json:"id"`
	ChannelID              uint      `json:"channel_id"`
	ModelName              string    `json:"model_name"`
	OldRatio               *float64  `json:"old_ratio,omitempty"`
	NewRatio               float64   `json:"new_ratio"`
	OldCompletionRatio     *float64  `json:"old_completion_ratio,omitempty"`
	NewCompletionRatio     float64   `json:"new_completion_ratio"`
	RawOldRatio            *float64  `json:"raw_old_ratio,omitempty"`
	RawNewRatio            float64   `json:"raw_new_ratio"`
	RawOldCompletionRatio  *float64  `json:"raw_old_completion_ratio,omitempty"`
	RawNewCompletionRatio  float64   `json:"raw_new_completion_ratio"`
	RechargeAdjusted       bool      `json:"recharge_adjusted"`
	RechargeMultiplier     *float64  `json:"recharge_multiplier,omitempty"`
	RechargeMultiplierMode string    `json:"recharge_multiplier_mode,omitempty"`
	ChangedAt              time.Time `json:"changed_at"`
}

func rateChangeOutputs(list []storage.RateChangeLog, channels []storage.Channel) []rateChangeOutput {
	channelMap := make(map[uint]storage.Channel, len(channels))
	for _, channelItem := range channels {
		channelMap[channelItem.ID] = channelItem
	}

	out := make([]rateChangeOutput, 0, len(list))
	for _, item := range list {
		view := rateChangeOutput{
			ID:                    item.ID,
			ChannelID:             item.ChannelID,
			ModelName:             item.ModelName,
			OldRatio:              copyFloat64(item.OldRatio),
			NewRatio:              item.NewRatio,
			OldCompletionRatio:    copyFloat64(item.OldCompletionRatio),
			NewCompletionRatio:    item.NewCompletionRatio,
			RawOldRatio:           copyFloat64(item.OldRatio),
			RawNewRatio:           item.NewRatio,
			RawOldCompletionRatio: copyFloat64(item.OldCompletionRatio),
			RawNewCompletionRatio: item.NewCompletionRatio,
			ChangedAt:             item.ChangedAt,
		}
		if channelItem, ok := channelMap[item.ChannelID]; ok && channelItem.RechargeMultiplier != nil && *channelItem.RechargeMultiplier > 0 {
			view.RechargeAdjusted = true
			view.RechargeMultiplier = copyFloat64(channelItem.RechargeMultiplier)
			view.RechargeMultiplierMode = connector.NormalizeRechargeMultiplierMode(channelItem.RechargeMultiplierMode)
			view.OldRatio = applyRechargeMultiplierToOptionalRatio(item.OldRatio, &channelItem)
			view.NewRatio = connector.ApplyRechargeMultiplier(item.NewRatio, channelItem.RechargeMultiplier, channelItem.RechargeMultiplierMode)
			view.OldCompletionRatio = applyRechargeMultiplierToOptionalRatio(item.OldCompletionRatio, &channelItem)
			view.NewCompletionRatio = connector.ApplyRechargeMultiplier(item.NewCompletionRatio, channelItem.RechargeMultiplier, channelItem.RechargeMultiplierMode)
		}
		out = append(out, view)
	}
	return out
}

func applyRechargeMultiplierToOptionalRatio(value *float64, channelItem *storage.Channel) *float64 {
	if value == nil {
		return nil
	}
	adjusted := connector.ApplyRechargeMultiplier(*value, channelItem.RechargeMultiplier, channelItem.RechargeMultiplierMode)
	return &adjusted
}

func copyFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func registerRates(g *gin.RouterGroup, d *Deps) {
	g.GET("/rates", func(c *gin.Context) { listRates(c, d) })
	g.GET("/rate-changes", func(c *gin.Context) {
		var channelID uint
		if s := c.Query("channel_id"); s != "" {
			id, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				fail(c, http.StatusBadRequest, err)
				return
			}
			channelID = uint(id)
		}
		page, pageSize, err := parsePageQuery(c)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		list, total, err := d.Rates.ListChangesPage(channelID, page, pageSize)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		var channels []storage.Channel
		if d.Channels != nil {
			channels, err = d.Channels.List()
			if err != nil {
				fail(c, http.StatusInternalServerError, err)
				return
			}
		}
		pages := 1
		if total > 0 {
			pages = int((total + int64(pageSize) - 1) / int64(pageSize))
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"items":     rateChangeOutputs(list, channels),
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"pages":     pages,
		}})
	})
}

func listRates(c *gin.Context, d *Deps) {
	channelIDs, err := parseChannelIDs(c.Query("channel_ids"))
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	list, err := d.Rates.ListByChannels(channelIDs)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	channels, err := d.Channels.List()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	channelMap := make(map[uint]*storage.Channel, len(channels))
	for i := range channels {
		channelMap[channels[i].ID] = &channels[i]
	}
	for i := range list {
		applyRechargeMultiplierToRates(list[i:i+1], channelMap[list[i].ChannelID])
	}

	connections := make(map[uint][]mainstation.RateConnection)
	if d.MainStation != nil {
		connections, err = d.MainStation.ListRateConnectionsForRates(list)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
	}
	classifier := rateranking.DefaultClassifier()
	if d.RateRanking != nil {
		classifier, err = d.RateRanking.Classifier(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": channelRateOutputs(list, connections, classifier)})
}

func parseChannelIDs(raw string) ([]uint, error) {
	const maxChannelIDs = 200
	parts := strings.Split(raw, ",")
	ids := make([]uint, 0, len(parts))
	seen := make(map[uint]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil || value == 0 {
			return nil, fmt.Errorf("渠道 ID %q 无效", part)
		}
		id := uint(value)
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) > maxChannelIDs {
			return nil, fmt.Errorf("渠道数量不能超过 %d", maxChannelIDs)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("至少需要一个渠道 ID")
	}
	return ids, nil
}
