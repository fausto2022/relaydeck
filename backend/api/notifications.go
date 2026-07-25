package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/fausto2022/relaydeck/backend/notify"
	"github.com/fausto2022/relaydeck/backend/storage"
	"github.com/gin-gonic/gin"
)

func registerNotifications(g *gin.RouterGroup, d *Deps) {
	gpc := g.Group("/notifications/channels")
	gpc.GET("", func(c *gin.Context) {
		list, err := d.Notifies.ListChannels()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		items := make([]gin.H, 0, len(list))
		for i := range list {
			item := gin.H{
				"id": list[i].ID, "name": list[i].Name, "type": list[i].Type,
				"subscriptions": list[i].Subscriptions, "enabled": list[i].Enabled,
				"proxy_enabled": list[i].ProxyEnabled, "created_at": list[i].CreatedAt, "updated_at": list[i].UpdatedAt,
			}
			if list[i].Type == storage.NotifyDingTalk {
				if config, configErr := decryptNotificationConfig(d, &list[i]); configErr == nil {
					item["display_config"] = gin.H{
						"message_style": config["message_style"],
						"action_url":    config["action_url"],
					}
				}
			}
			items = append(items, item)
		}
		c.JSON(http.StatusOK, gin.H{"data": items})
	})
	gpc.POST("", func(c *gin.Context) { createNotifyChannel(c, d) })
	gpc.PUT("/:id", func(c *gin.Context) { updateNotifyChannel(c, d) })
	gpc.DELETE("/:id", func(c *gin.Context) {
		id, err := uintParam(c, "id")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := d.Notifies.DeleteChannel(id); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	gpc.POST("/:id/test", func(c *gin.Context) { testNotify(c, d) })

	g.GET("/notifications/logs", func(c *gin.Context) {
		page, pageSize, err := parsePageQuery(c)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		list, total, err := d.Notifies.ListLogsPage(page, pageSize)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		channels, err := d.Notifies.ListChannels()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		channelMeta := make(map[uint]gin.H, len(channels))
		for _, ch := range channels {
			channelMeta[ch.ID] = gin.H{
				"channel_name": ch.Name,
				"channel_type": ch.Type,
			}
		}
		items := make([]gin.H, 0, len(list))
		for _, item := range list {
			meta := channelMeta[item.ChannelID]
			row := gin.H{
				"id":                  item.ID,
				"channel_id":          item.ChannelID,
				"upstream_channel_id": item.UpstreamChannelID,
				"event":               item.Event,
				"subject":             item.Subject,
				"body":                item.Body,
				"success":             item.Success,
				"error_message":       item.ErrorMessage,
				"sent_at":             item.SentAt,
			}
			for k, v := range meta {
				row[k] = v
			}
			items = append(items, row)
		}
		pages := 1
		if total > 0 {
			pages = int((total + int64(pageSize) - 1) / int64(pageSize))
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"pages":     pages,
		}})
	})

	g.GET("/notifications/events", func(c *gin.Context) {
		page, pageSize, err := parsePageQuery(c)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		list, total, err := d.Notifies.ListEventsPage(page, pageSize)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		channels, err := d.Channels.List()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		channelNames := make(map[uint]string, len(channels))
		for _, channel := range channels {
			channelNames[channel.ID] = channel.Name
		}
		items := make([]gin.H, 0, len(list))
		for _, item := range list {
			items = append(items, gin.H{
				"id":                    item.ID,
				"upstream_channel_id":   item.UpstreamChannelID,
				"upstream_channel_name": channelNames[item.UpstreamChannelID],
				"event":                 item.Event,
				"subject":               item.Subject,
				"body":                  item.Body,
				"created_at":            item.CreatedAt,
			})
		}
		pages := 1
		if total > 0 {
			pages = int((total + int64(pageSize) - 1) / int64(pageSize))
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"items": items, "total": total, "page": page, "page_size": pageSize, "pages": pages,
		}})
	})
}

type notifyChannelInput struct {
	Name          string                          `json:"name" binding:"required"`
	Type          storage.NotificationChannelType `json:"type" binding:"required"`
	Config        string                          `json:"config"` // JSON string；编辑时可留空保留原值
	Subscriptions string                          `json:"subscriptions"`
	Enabled       bool                            `json:"enabled"`
	ProxyEnabled  bool                            `json:"proxy_enabled"`
}

// normalizeSubscriptions 把输入的订阅 JSON 字符串规整为 "[]" 或合法订阅规则数组。
// 解析失败返回错误以便 API 返回 400。
func normalizeSubscriptions(raw string) (string, error) {
	if raw == "" || raw == "null" {
		return "[]", nil
	}
	var list []notify.Subscription
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return "", err
	}
	out, err := json.Marshal(list)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func createNotifyChannel(c *gin.Context, d *Deps) {
	var in notifyChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if in.Config == "" {
		fail(c, http.StatusBadRequest, errors.New("config is required"))
		return
	}
	subs, err := normalizeSubscriptions(in.Subscriptions)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	cipherCfg, err := d.Cipher.Encrypt(in.Config)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	ch := &storage.NotificationChannel{
		Name:          in.Name,
		Type:          in.Type,
		ConfigCipher:  cipherCfg,
		Subscriptions: subs,
		Enabled:       in.Enabled,
		ProxyEnabled:  in.ProxyEnabled,
	}
	if err := d.Notifies.CreateChannel(ch); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ch})
}

func updateNotifyChannel(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	ch, err := d.Notifies.FindChannel(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	var in notifyChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	subs, err := normalizeSubscriptions(in.Subscriptions)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	ch.Name = in.Name
	ch.Type = in.Type
	ch.Enabled = in.Enabled
	ch.ProxyEnabled = in.ProxyEnabled
	ch.Subscriptions = subs
	if in.Config != "" {
		config := in.Config
		if ch.Type == storage.NotifyDingTalk {
			config, err = mergeNotificationConfig(d, ch, in.Config)
			if err != nil {
				fail(c, http.StatusBadRequest, err)
				return
			}
		}
		cipherCfg, err := d.Cipher.Encrypt(config)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ch.ConfigCipher = cipherCfg
	}
	if err := d.Notifies.UpdateChannel(ch); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ch})
}

func decryptNotificationConfig(d *Deps, ch *storage.NotificationChannel) (map[string]any, error) {
	if d == nil || d.Cipher == nil {
		return nil, errors.New("notification cipher is unavailable")
	}
	raw, err := d.Cipher.Decrypt(ch.ConfigCipher)
	if err != nil {
		return nil, err
	}
	config := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	return config, nil
}

func mergeNotificationConfig(d *Deps, ch *storage.NotificationChannel, raw string) (string, error) {
	config, err := decryptNotificationConfig(d, ch)
	if err != nil {
		return "", err
	}
	patch := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		return "", err
	}
	for key, value := range patch {
		if text, ok := value.(string); ok && text == "" && (key == "webhook_url" || key == "secret") {
			continue
		}
		config[key] = value
	}
	merged, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}

func testNotify(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	ch, err := d.Notifies.FindChannel(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	msg := notify.Message{
		Subject: "测试通知",
		Body: notify.MarkdownDetails(
			"通知渠道测试成功。",
			notify.Detail("渠道名称", ch.Name),
			notify.Detail("渠道类型", ch.Type),
			notify.Detail("发送时间", time.Now().Format("2006-01-02 15:04:05")),
		) + notify.MarkdownNote("说明", "收到此消息表示 RelayDeck 已能正常调用该通知渠道。"),
	}
	if err := d.Dispatcher.Send(c.Request.Context(), ch, msg); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
