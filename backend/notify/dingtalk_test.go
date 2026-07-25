package notify

import (
	"testing"

	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestDingTalkPayloadUsesActionCardWhenConfigured(t *testing.T) {
	notifier := &dingTalk{cfg: dingTalkConfig{
		MessageStyle: "action_card",
		ActionURL:    "https://relaydeck.example.com",
	}}
	payload := notifier.payload(Message{
		Event:   storage.EventMainPoolUnavailable,
		Subject: "[RelayDeck] 主站分组无可用账号",
		Body:    MarkdownDetails("主站分组可用账号数量发生变化。", Detail("账号池", "默认分组")),
	})
	if payload["msgtype"] != "actionCard" {
		t.Fatalf("msgtype = %#v", payload["msgtype"])
	}
	card, ok := payload["actionCard"].(map[string]any)
	if !ok || card["singleURL"] != "https://relaydeck.example.com" || card["singleTitle"] != "打开 RelayDeck" {
		t.Fatalf("action card = %#v", payload["actionCard"])
	}
}

func TestDingTalkPayloadFallsBackToMarkdownWithoutActionURL(t *testing.T) {
	notifier := &dingTalk{cfg: dingTalkConfig{MessageStyle: "action_card"}}
	payload := notifier.payload(Message{Subject: "[RelayDeck] 测试通知", Body: "通知渠道测试成功。"})
	if payload["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %#v", payload["msgtype"])
	}
}
