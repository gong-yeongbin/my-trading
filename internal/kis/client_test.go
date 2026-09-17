package kis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeKIS 는 한투 서버 흉내. 경로별 핸들러를 등록해 쓴다.
type fakeKIS struct {
	srv        *httptest.Server
	mu         sync.Mutex
	tokenCalls int
	tokenValue string
	handlers   map[string]http.HandlerFunc
	client     *Client
}

func newFakeKIS(t *testing.T) *fakeKIS {
	t.Helper()
	f := &fakeKIS{tokenValue: "tok-1", handlers: map[string]http.HandlerFunc{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/tokenP", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.tokenCalls++
		tok := f.tokenValue
		f.mu.Unlock()
		var body struct {
			GrantType string `json:"grant_type"`
			AppKey    string `json:"appkey"`
			AppSecret string `json:"appsecret"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if r.Method != http.MethodPost || body.GrantType != "client_credentials" || body.AppKey != "AK" || body.AppSecret != "AS" {
			http.Error(w, `{"error_description":"bad request"}`, http.StatusBadRequest)
			return
		}
		exp := time.Now().In(kst).Add(24 * time.Hour).Format("2006-01-02 15:04:05")
		json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "access_token_token_expired": exp, "token_type": "Bearer", "expires_in": 86400})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		h, ok := f.handlers[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	f.client = New(f.srv.URL, "AK", "AS", filepath.Join(t.TempDir(), "token.json"), 1000)
	f.client.retryWait = 0
	return f
}

func (f *fakeKIS) handle(path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[path] = h
}

func (f *fakeKIS) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls
}

func okJSON(w http.ResponseWriter, payload string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(payload))
}

func TestTokenIssuedOnceAndCached(t *testing.T) {
	f := newFakeKIS(t)
	ctx := context.Background()
	tok, err := f.client.Token(ctx)
	if err != nil || tok != "tok-1" {
		t.Fatalf("Token = %q err=%v", tok, err)
	}
	if _, err := f.client.Token(ctx); err != nil {
		t.Fatal(err)
	}
	if f.calls() != 1 {
		t.Errorf("token issued %d times, want 1", f.calls())
	}
	// 새 클라이언트가 같은 캐시 파일을 쓰면 재발급하지 않는다
	c2 := New(f.srv.URL, "AK", "AS", f.client.TokenCachePath, 1000)
	if tok, err := c2.Token(ctx); err != nil || tok != "tok-1" {
		t.Errorf("cached token not reused: %q %v", tok, err)
	}
	if f.calls() != 1 {
		t.Errorf("token issued %d times after cache reuse, want 1", f.calls())
	}
	if info, err := os.Stat(f.client.TokenCachePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("token cache perm = %v err=%v", info.Mode().Perm(), err)
	}
}

func TestTokenReissuedWhenExpired(t *testing.T) {
	f := newFakeKIS(t)
	ctx := context.Background()
	if _, err := f.client.Token(ctx); err != nil {
		t.Fatal(err)
	}
	f.client.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	f.mu.Lock()
	f.tokenValue = "tok-2"
	f.mu.Unlock()
	tok, err := f.client.Token(ctx)
	if err != nil || tok != "tok-2" || f.calls() != 2 {
		t.Errorf("expired token not reissued: %q calls=%d err=%v", tok, f.calls(), err)
	}
}

func TestGetSendsHeadersAndDecodes(t *testing.T) {
	f := newFakeKIS(t)
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Bearer tok-1" || r.Header.Get("appkey") != "AK" ||
			r.Header.Get("appsecret") != "AS" || r.Header.Get("tr_id") != "TR1" || r.Header.Get("custtype") != "P" {
			http.Error(w, "bad headers", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("X") != "1" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		okJSON(w, `{"rt_cd":"0","msg_cd":"MCA00000","msg1":"ok","output2":[{"a":"b"}]}`)
	})
	var out struct {
		Output2 []map[string]string `json:"output2"`
	}
	err := f.client.get(context.Background(), "/ping", "TR1", url.Values{"X": {"1"}}, &out)
	if err != nil || len(out.Output2) != 1 || out.Output2[0]["a"] != "b" {
		t.Errorf("get: err=%v out=%+v", err, out)
	}
}

func TestGetRefreshesTokenOn401Once(t *testing.T) {
	f := newFakeKIS(t)
	var n int
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		n++
		if r.Header.Get("authorization") == "Bearer tok-1" {
			f.mu.Lock()
			f.tokenValue = "tok-2"
			f.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			okJSON(w, `{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기간이 만료된 token"}`)
			return
		}
		okJSON(w, `{"rt_cd":"0","msg_cd":"MCA00000","msg1":"ok"}`)
	})
	var out struct{}
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err != nil {
		t.Fatalf("expected success after refresh: %v", err)
	}
	if n != 2 || f.calls() != 2 {
		t.Errorf("requests=%d tokenCalls=%d, want 2 and 2", n, f.calls())
	}
	// 재발급 후에도 401 이면 포기
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		okJSON(w, `{"rt_cd":"1","msg_cd":"EGW00123","msg1":"만료"}`)
	})
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err == nil || !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized after second 401, got %v", err)
	}
}

// TestGetRefreshesTokenOnTokenErrorCode: msg_cd 가 EGW00121/EGW00123 이면 HTTP 상태가 401 이
// 아니어도(여기선 500) 토큰을 재발급해 재시도한다.
func TestGetRefreshesTokenOnTokenErrorCode(t *testing.T) {
	f := newFakeKIS(t)
	var n int
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		n++
		if r.Header.Get("authorization") == "Bearer tok-1" {
			f.mu.Lock()
			f.tokenValue = "tok-2"
			f.mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			okJSON(w, `{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기간이 만료된 token"}`)
			return
		}
		okJSON(w, `{"rt_cd":"0","msg_cd":"MCA00000","msg1":"ok"}`)
	})
	var out struct{}
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err != nil {
		t.Fatalf("expected success after refresh: %v", err)
	}
	if n != 2 || f.calls() != 2 {
		t.Errorf("requests=%d tokenCalls=%d, want 2 and 2", n, f.calls())
	}
	// 재발급 후에도 같은 오류면 포기
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		okJSON(w, `{"rt_cd":"1","msg_cd":"EGW00121","msg1":"유효하지 않은 token"}`)
	})
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err == nil || !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized after second failure, got %v", err)
	}
}

func TestGetRetriesRateLimitThenFails(t *testing.T) {
	f := newFakeKIS(t)
	var n int
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			okJSON(w, `{"rt_cd":"1","msg_cd":"EGW00201","msg1":"초당 거래건수를 초과하였습니다."}`)
			return
		}
		okJSON(w, `{"rt_cd":"0","msg_cd":"MCA00000","msg1":"ok"}`)
	})
	var out struct{}
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err != nil || n != 3 {
		t.Errorf("rate limit retry: err=%v n=%d", err, n)
	}
	n = 0
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(http.StatusInternalServerError)
		okJSON(w, `{"rt_cd":"1","msg_cd":"EGW00201","msg1":"초당 거래건수를 초과하였습니다."}`)
	})
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err == nil || n != 4 {
		t.Errorf("expected failure after 1+3 attempts: err=%v n=%d", err, n)
	}
}

func TestGetReportsBusinessError(t *testing.T) {
	f := newFakeKIS(t)
	f.handle("/ping", func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, `{"rt_cd":"1","msg_cd":"OPSQ0001","msg1":"조회할 자료가 없습니다"}`)
	})
	var out struct{}
	err := f.client.get(context.Background(), "/ping", "TR1", nil, &out)
	if err == nil || !contains(err.Error(), "OPSQ0001") {
		t.Errorf("expected msg_cd in error, got %v", err)
	}
}

func TestLimiterSpacesCalls(t *testing.T) {
	l := newLimiter(50) // 20ms 간격
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := l.wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 55*time.Millisecond {
		t.Errorf("4 calls at 50/s took %v, want >= 60ms", el)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := newLimiter(1).waitTwice(cctx); err == nil {
		t.Error("expected ctx error")
	}
}

func (l *limiter) waitTwice(ctx context.Context) error {
	if err := l.wait(ctx); err != nil {
		return err
	}
	return l.wait(ctx)
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
