package auth

import (
	"testing"
	"time"
)

func TestSessionVersionRevokesPreviousTokens(t *testing.T) {
	oldService, err := New("admin", "old-password", "token-secret", time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	oldToken, _, err := oldService.Login("admin", "old-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	updatedService, err := NewWithSessionVersion("admin", "new-password", "token-secret", time.Hour, 1)
	if err != nil {
		t.Fatalf("NewWithSessionVersion: %v", err)
	}
	if _, err := updatedService.Verify(oldToken); err == nil {
		t.Fatal("old token remains valid after session version change")
	}
	if updatedService.VerifyPassword("old-password") {
		t.Fatal("old password remains valid after credential change")
	}
	if !updatedService.VerifyPassword("new-password") {
		t.Fatal("new password was not accepted")
	}

	newToken, _, err := updatedService.Login("admin", "new-password")
	if err != nil {
		t.Fatalf("Login with new password: %v", err)
	}
	if subject, err := updatedService.Verify(newToken); err != nil || subject != "admin" {
		t.Fatalf("Verify new token = %q, %v", subject, err)
	}
}

func TestNewKeepsLegacySessionVersion(t *testing.T) {
	service, err := New("admin", "password", "token-secret", time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token, _, err := service.Login("admin", "password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := service.Verify(token); err != nil {
		t.Fatalf("Verify legacy token: %v", err)
	}
}
