package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fausto2022/relaydeck/backend/auth"
	"github.com/fausto2022/relaydeck/backend/config"
	"github.com/fausto2022/relaydeck/backend/runtimeconfig"
	"github.com/fausto2022/relaydeck/backend/scheduler"
	"github.com/gin-gonic/gin"
)

func TestChangeCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, &config.Config{
		Auth: config.AuthConfig{
			Enabled:         true,
			Username:        "admin",
			Password:        "old-password",
			TokenSecret:     "token-secret",
			SessionTTLHours: 1,
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	authSvc, err := auth.New("admin", "old-password", "token-secret", time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	mgr := runtimeconfig.New(
		path,
		"token-secret",
		log,
		nil,
		nil,
		authSvc,
		nil,
		config.ProxyConfig{},
		config.UpstreamConfig{},
		func(scfg config.SchedulerConfig, pcfg config.ProxyConfig) *scheduler.Scheduler {
			return scheduler.New(scfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, pcfg, log)
		},
	)

	r := gin.New()
	api := r.Group("/api")
	registerAuth(api, &Deps{Runtime: mgr})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/change-credentials", strings.NewReader(`{
		"current_password":"old-password",
		"username":"new-admin",
		"new_password":"new-password"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	updated, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if updated.Auth.Username != "new-admin" || updated.Auth.Password != "new-password" || updated.Auth.SessionVersion != 1 {
		t.Fatalf("auth config = %#v", updated.Auth)
	}
	if _, _, err := mgr.CurrentAuth().Login("admin", "old-password"); err == nil {
		t.Fatal("old credentials remain valid")
	}
	if _, _, err := mgr.CurrentAuth().Login("new-admin", "new-password"); err != nil {
		t.Fatalf("new credentials rejected: %v", err)
	}
}

func TestChangeCredentialsRejectsWrongCurrentPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, &config.Config{Auth: config.AuthConfig{
		Enabled: true, Username: "admin", Password: "old-password", TokenSecret: "token-secret",
	}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	authSvc, err := auth.New("admin", "old-password", "token-secret", time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := runtimeconfig.New(path, "token-secret", log, nil, nil, authSvc, nil, config.ProxyConfig{}, config.UpstreamConfig{}, func(scfg config.SchedulerConfig, pcfg config.ProxyConfig) *scheduler.Scheduler {
		return scheduler.New(scfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, pcfg, log)
	})
	r := gin.New()
	registerAuth(r.Group("/api"), &Deps{Runtime: mgr})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/change-credentials", strings.NewReader(`{
		"current_password":"wrong-password",
		"username":"new-admin",
		"new_password":"new-password"
	}`))
	request.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
