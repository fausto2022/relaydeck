package config

import (
	"path/filepath"
	"testing"

	"github.com/fausto2022/relaydeck/backend/storage"
)

func TestLoadAppliesUpstreamDefaults(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Upstream.TimeoutSeconds != DefaultUpstreamTimeoutSeconds {
		t.Fatalf("timeout seconds = %d", cfg.Upstream.TimeoutSeconds)
	}
	if cfg.Upstream.UserAgent != DefaultUpstreamUserAgent {
		t.Fatalf("user agent = %q", cfg.Upstream.UserAgent)
	}
}

func TestUpstreamConfigWithDefaultsKeepsCustomUserAgent(t *testing.T) {
	cfg := UpstreamConfig{
		TimeoutSeconds: 0,
		UserAgent:      "custom-agent",
	}.WithDefaults()
	if cfg.TimeoutSeconds != DefaultUpstreamTimeoutSeconds {
		t.Fatalf("timeout seconds = %d", cfg.TimeoutSeconds)
	}
	if cfg.UserAgent != "custom-agent" {
		t.Fatalf("user agent = %q", cfg.UserAgent)
	}
}

func TestNotificationDisabledEventsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := []storage.NotificationEvent{storage.EventBalanceLow, storage.EventMainMemberDisabled}
	if err := Save(path, &Config{Notifications: NotificationsConfig{DisabledEvents: want}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Notifications.DisabledEvents) != len(want) {
		t.Fatalf("disabled events = %#v", cfg.Notifications.DisabledEvents)
	}
	for i := range want {
		if cfg.Notifications.DisabledEvents[i] != want[i] {
			t.Fatalf("disabled events = %#v", cfg.Notifications.DisabledEvents)
		}
	}
}
