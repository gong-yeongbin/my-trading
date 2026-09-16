package ls

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer 는 /oauth2/token 만 처리하는 가짜 서버. 요청 수를 센다.
func tokenServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		for k, want := range map[string]string{"appkey": "k", "appsecretkey": "s", "grant_type": "client_credentials", "scope": "oob"} {
			if got := r.PostForm.Get(k); got != want {
				t.Errorf("form %s = %q, want %q", k, got, want)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &n
}

func newTestClient(t *testing.T, srv *httptest.Server, cache string) *Client {
	t.Helper()
	return New(Config{BaseURL: srv.URL, WSURL: "ws://unused", AppKey: "k", AppSecret: "s", TokenCache: cache}, slog.New(slog.DiscardHandler))
}

func TestTokenRequestAndCache(t *testing.T) {
	srv, n := tokenServer(t, 200, `{"access_token":"tok1","expires_in":3600,"token_type":"Bearer"}`)
	cache := filepath.Join(t.TempDir(), "sub", "ls_token.json")
	c := newTestClient(t, srv, cache)

	tok, err := c.Token(context.Background())
	if err != nil || tok != "tok1" {
		t.Fatalf("Token = %q, %v", tok, err)
	}
	if _, err = c.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Errorf("second Token() should use memory cache, requests = %d", n.Load())
	}
	b, err := os.ReadFile(cache)
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	var saved cachedToken
	if err := json.Unmarshal(b, &saved); err != nil || saved.AccessToken != "tok1" || time.Until(saved.ExpiresAt) < 3500*time.Second {
		t.Errorf("cache content = %s, err %v", b, err)
	}

	// 새 클라이언트는 파일 캐시를 읽고 요청하지 않는다.
	c2 := newTestClient(t, srv, cache)
	if tok, err := c2.Token(context.Background()); err != nil || tok != "tok1" {
		t.Fatalf("cached Token = %q, %v", tok, err)
	}
	if n.Load() != 1 {
		t.Errorf("file cache not used, requests = %d", n.Load())
	}
}

func TestTokenRefreshWhenExpiring(t *testing.T) {
	srv, n := tokenServer(t, 200, `{"access_token":"tok2","expires_in":3600}`)
	cache := filepath.Join(t.TempDir(), "ls_token.json")
	old, _ := json.Marshal(cachedToken{AccessToken: "old", ExpiresAt: time.Now().Add(10 * time.Second)})
	if err := os.WriteFile(cache, old, 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, srv, cache)
	tok, err := c.Token(context.Background())
	if err != nil || tok != "tok2" {
		t.Fatalf("Token = %q, %v (want refresh, cache expires in 10s < 30s margin)", tok, err)
	}
	if n.Load() != 1 {
		t.Errorf("requests = %d", n.Load())
	}
}

func TestTokenDefaultExpiry(t *testing.T) {
	srv, _ := tokenServer(t, 200, `{"access_token":"tok3"}`)
	c := newTestClient(t, srv, filepath.Join(t.TempDir(), "t.json"))
	if _, err := c.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if until := time.Until(c.expiresAt); until < 86000*time.Second {
		t.Errorf("missing expires_in should default to 86400s, got %v", until)
	}
}

func TestTokenErrors(t *testing.T) {
	srv, _ := tokenServer(t, 401, `{"error":"invalid_client"}`)
	c := newTestClient(t, srv, filepath.Join(t.TempDir(), "t.json"))
	if _, err := c.Token(context.Background()); err == nil {
		t.Error("expected error on 401")
	}
	srv2, _ := tokenServer(t, 200, `{"token_type":"Bearer"}`)
	c2 := newTestClient(t, srv2, filepath.Join(t.TempDir(), "t.json"))
	if _, err := c2.Token(context.Background()); err == nil {
		t.Error("expected error when access_token missing")
	}
}
