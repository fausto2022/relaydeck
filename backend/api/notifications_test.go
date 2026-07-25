package api

import (
	"encoding/json"
	"testing"

	"github.com/fausto2022/relaydeck/backend/crypto"
	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestMergeDingTalkConfigPreservesCredentials(t *testing.T) {
	cipher, err := crypto.NewCipher("test-secret")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	encrypted, err := cipher.Encrypt(`{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=token","secret":"SECxxx","message_style":"markdown"}`)
	if err != nil {
		t.Fatalf("encrypt config: %v", err)
	}
	channel := &storage.NotificationChannel{Type: storage.NotifyDingTalk, ConfigCipher: encrypted}
	merged, err := mergeNotificationConfig(&Deps{Cipher: cipher}, channel, `{"webhook_url":"","secret":"","message_style":"action_card","action_url":"https://relaydeck.example.com"}`)
	if err != nil {
		t.Fatalf("merge config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(merged), &config); err != nil {
		t.Fatalf("decode merged config: %v", err)
	}
	if config["webhook_url"] != "https://oapi.dingtalk.com/robot/send?access_token=token" || config["secret"] != "SECxxx" {
		t.Fatalf("credentials changed: %#v", config)
	}
	if config["message_style"] != "action_card" || config["action_url"] != "https://relaydeck.example.com" {
		t.Fatalf("card config = %#v", config)
	}
}
