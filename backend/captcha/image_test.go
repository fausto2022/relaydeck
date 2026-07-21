package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestSolveImageTaskStripsDataPrefixAndNormalizesText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/createTask" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body struct {
			ClientKey string `json:"clientKey"`
			Task      struct {
				Type   string `json:"type"`
				Body   string `json:"body"`
				Module string `json:"module"`
			} `json:"task"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.ClientKey != "key" || body.Task.Type != "ImageToTextTask" || body.Task.Body != "aW1hZ2U=" || body.Task.Module != "common" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"errorId":0,"status":"ready","solution":{"text":" ztv-hd "}}`))
	}))
	defer server.Close()

	text, err := solveImageTask(
		context.Background(),
		resty.NewWithClient(server.Client()),
		server.URL,
		"key",
		"test",
		"data:image/png;base64,aW1hZ2U=",
		true,
	)
	if err != nil {
		t.Fatalf("solveImageTask: %v", err)
	}
	if text != "ZTVHD" {
		t.Fatalf("text = %q, want ZTVHD", text)
	}
}
