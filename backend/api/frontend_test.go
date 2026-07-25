package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestFrontendCacheControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dist := fstest.MapFS{
		"index.html":               {Data: []byte("<html>app</html>")},
		"assets/index-abc12345.js": {Data: []byte("console.log('app')")},
	}
	router := gin.New()
	registerFrontend(router, dist)

	tests := []struct {
		name         string
		path         string
		wantCache    string
		wantResponse string
	}{
		{
			name:         "hashed asset is immutable",
			path:         "/assets/index-abc12345.js",
			wantCache:    "public, max-age=31536000, immutable",
			wantResponse: "console.log('app')",
		},
		{
			name:         "index is revalidated",
			path:         "/",
			wantCache:    "no-cache",
			wantResponse: "<html>app</html>",
		},
		{
			name:         "spa fallback is revalidated",
			path:         "/settings",
			wantCache:    "no-cache",
			wantResponse: "<html>app</html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Fatalf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if got := recorder.Body.String(); got != tt.wantResponse {
				t.Fatalf("body = %q, want %q", got, tt.wantResponse)
			}
		})
	}
}
