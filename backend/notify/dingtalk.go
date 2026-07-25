package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/fausto2022/relaydeck/backend/storage"
	"github.com/go-resty/resty/v2"
)

func init() {
	Register(storage.NotifyDingTalk, func(raw string) (Notifier, error) { return newDingTalk(raw) })
}

type dingTalkConfig struct {
	WebhookURL   string `json:"webhook_url"`
	Secret       string `json:"secret,omitempty"`
	MessageStyle string `json:"message_style,omitempty"`
	ActionURL    string `json:"action_url,omitempty"`
}

type dingTalk struct {
	cfg  dingTalkConfig
	http *resty.Client
}

func newDingTalk(raw string) (*dingTalk, error) {
	var cfg dingTalkConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	if cfg.WebhookURL == "" {
		return nil, errors.New("dingtalk webhook_url is required")
	}
	return &dingTalk{cfg: cfg, http: newNotificationHTTPClient()}, nil
}

func (d *dingTalk) Type() storage.NotificationChannelType { return storage.NotifyDingTalk }

func (d *dingTalk) SetProxy(proxyURL string) {
	if proxyURL != "" {
		d.http.SetProxy(proxyURL)
	}
}

func (d *dingTalk) Send(ctx context.Context, msg Message) error {
	endpoint := d.cfg.WebhookURL
	if d.cfg.Secret != "" {
		ts := time.Now().UnixMilli()
		stringToSign := fmt.Sprintf("%d\n%s", ts, d.cfg.Secret)
		mac := hmac.New(sha256.New, []byte(d.cfg.Secret))
		mac.Write([]byte(stringToSign))
		sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		endpoint = fmt.Sprintf("%s&timestamp=%d&sign=%s", endpoint, ts, url.QueryEscape(sign))
	}
	payload := d.payload(msg)
	resp, err := d.http.R().
		SetContext(ctx).
		SetBody(payload).
		Post(endpoint)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return errors.New("dingtalk returned " + resp.Status())
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("dingtalk returned errcode %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

func (d *dingTalk) payload(msg Message) map[string]any {
	text := dingTalkCardText(msg)
	if strings.EqualFold(strings.TrimSpace(d.cfg.MessageStyle), "action_card") && strings.TrimSpace(d.cfg.ActionURL) != "" {
		return map[string]any{
			"msgtype": "actionCard",
			"actionCard": map[string]any{
				"title":          msg.Subject,
				"text":           text,
				"singleTitle":    "打开 RelayDeck",
				"singleURL":      strings.TrimSpace(d.cfg.ActionURL),
				"btnOrientation": "0",
			},
		}
	}
	return map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Subject,
			"text":  text,
		},
	}
}

func dingTalkCardText(msg Message) string {
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(strings.TrimSpace(msg.Subject))
	b.WriteString("\n\n---\n\n")
	b.WriteString(strings.TrimSpace(msg.Body))
	b.WriteString("\n\n---\n\n")
	b.WriteString("> RelayDeck 自动通知 · ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	return b.String()
}
