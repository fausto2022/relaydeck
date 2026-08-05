package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestClientErrorReportWritesStructuredLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	router := gin.New()
	registerClientErrors(router.Group("/api"), &Deps{
		Log: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/client-errors", strings.NewReader(`{
		"name":"NotFoundError",
		"message":"removeChild failed",
		"stack":"stack trace",
		"component_stack":"component trace",
		"url":"https://relay.example/main-station",
		"document_language":"en",
		"dom_mismatch":true,
		"translation_detected":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	output := logs.String()
	for _, expected := range []string{"frontend application error", "removeChild failed", "dom_mismatch=true", "translation_detected=true"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("log %q does not contain %q", output, expected)
		}
	}
}

func TestClientErrorReportRejectsInvalidOrOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerClientErrors(router.Group("/api"), &Deps{})
	for name, body := range map[string]string{
		"empty message": `{"message":" "}`,
		"oversized":     `{"message":"` + strings.Repeat("x", int(clientErrorBodyLimit)) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/client-errors", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}
