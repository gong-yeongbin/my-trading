# 5단계: 한투 클라이언트 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 한투 Open API 토큰 발급·캐시, 초당 호출 제한, 401·초당 제한 재시도, 종목 일봉(100건 분할)과 지수 일봉 조회를 만든다. 실서버는 호출하지 않고 전부 `httptest`로 검증한다.

**Architecture:** `kis`(Client, 토큰, limiter, chart). 1단계 `config.KISConfig`와 4단계 `data` 타입을 소비한다.

**Tech Stack:** Go 1.27, 표준 `net/http`. 테스트는 표준 `testing` + `httptest`.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (7.1~7.4절, 11절의 401·초당 제한 항목, 12절의 kis 항목)

**선행:** 4단계(SQLite 저장소) 완료.

## Global Constraints

- 모듈 경로: `github.com/gong-yeongbin/my-trading`
- 외부 의존성은 위 3개로 제한. 테스트 프레임워크(testify 등) 추가 금지.
- 가격은 `int64` 원 단위, 지수는 `float64`. 날짜는 `YYYY-MM-DD` 문자열로 저장하고 `time.Time`은 KST 자정.
- 한투 실서버는 테스트에서 호출하지 않는다. 마지막 수동 확인에서만 호출.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/token.json`은 읽기 금지. 값이 필요하면 프로그램이 읽게 하고 Claude는 열지 않는다.
- 파일 편집 후 `gofmt -l .` 결과가 비어 있어야 한다.

---


## 파일 구조

```
internal/kis/client.go        Client, get(), 401·EGW00201 재시도
internal/kis/token.go         토큰 발급·캐시
internal/kis/limiter.go       초당 호출 제한
internal/kis/chart.go         DailyBars, IndexBars, 기간 분할
internal/kis/client_test.go   가짜 서버 헬퍼 + 토큰/재시도 테스트
internal/kis/chart_test.go
```

---

### Task 1: 한투 클라이언트 기본 (토큰, 호출 제한, 재시도)

**Files:**
- Create: `internal/kis/client.go`, `internal/kis/token.go`, `internal/kis/limiter.go`
- Test: `internal/kis/client_test.go`

**Interfaces:**
- Produces: `kis.New(baseURL, appKey, appSecret, tokenCachePath string, rps float64) *Client`, `(*Client).Token(ctx) (string, error)`, 비공개 `(*Client).get(ctx, path, trID string, params url.Values, out any) error`. 테스트용 `newFakeKIS(t) *fakeKIS` 헬퍼는 Task 2·3 테스트가 재사용한다.
- 응답 봉투: `rt_cd`가 `"0"`이면 성공. `msg_cd == "EGW00201"`이면 초당 제한 초과.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/kis/client_test.go`:

```go
package kis

import (
	"context"
	"encoding/json"
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
	if err := f.client.get(context.Background(), "/ping", "TR1", nil, &out); err == nil {
		t.Error("expected error after second 401")
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
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/kis/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: New`, `kst` 등)

- [ ] **Step 3: 구현**

`internal/kis/limiter.go`:

```go
package kis

import (
	"context"
	"sync"
	"time"
)

// limiter 는 호출 사이에 최소 간격을 강제한다 (초당 rps 건).
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func newLimiter(rps float64) *limiter {
	return &limiter{interval: time.Duration(float64(time.Second) / rps)}
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	var delay time.Duration
	if l.next.After(now) {
		delay = l.next.Sub(now)
		l.next = l.next.Add(l.interval)
	} else {
		l.next = now.Add(l.interval)
	}
	l.mu.Unlock()
	if delay == 0 {
		return ctx.Err()
	}
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

`internal/kis/token.go`:

```go
package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var kst = time.FixedZone("KST", 9*3600)

const tokenExpiryLayout = "2006-01-02 15:04:05"

type cachedToken struct {
	AccessToken string `json:"access_token"`
	Expired     string `json:"expired"` // KST, tokenExpiryLayout
}

func (t cachedToken) validAt(now time.Time) bool {
	exp, err := time.ParseInLocation(tokenExpiryLayout, t.Expired, kst)
	if err != nil {
		return false
	}
	return now.Add(10 * time.Minute).Before(exp)
}

// Token 은 캐시 파일의 토큰이 유효하면 그대로 쓰고, 아니면 새로 발급해 저장한다.
// 재발급은 분당 1회 제한이 있으므로 매 호출마다 발급하면 안 된다.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.tok != nil && c.tok.validAt(now) {
		return c.tok.AccessToken, nil
	}
	if t, err := c.readTokenCache(); err == nil && t.validAt(now) {
		c.tok = &t
		return t.AccessToken, nil
	}
	t, err := c.issueToken(ctx)
	if err != nil {
		return "", err
	}
	if err := c.writeTokenCache(t); err != nil {
		return "", err
	}
	c.tok = &t
	return t.AccessToken, nil
}

// invalidateToken 은 401 을 받았을 때 다음 Token() 이 재발급하도록 만든다.
func (c *Client) invalidateToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tok = nil
	os.Remove(c.TokenCachePath)
}

func (c *Client) issueToken(ctx context.Context) (cachedToken, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "client_credentials", "appkey": c.AppKey, "appsecret": c.AppSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/oauth2/tokenP", bytes.NewReader(body))
	if err != nil {
		return cachedToken{}, err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return cachedToken{}, fmt.Errorf("kis: token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return cachedToken{}, fmt.Errorf("kis: token HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	var r struct {
		AccessToken string `json:"access_token"`
		Expired     string `json:"access_token_token_expired"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.AccessToken == "" {
		return cachedToken{}, fmt.Errorf("kis: token response: %s", bytes.TrimSpace(raw))
	}
	return cachedToken{AccessToken: r.AccessToken, Expired: r.Expired}, nil
}

func (c *Client) readTokenCache() (cachedToken, error) {
	raw, err := os.ReadFile(c.TokenCachePath)
	if err != nil {
		return cachedToken{}, err
	}
	var t cachedToken
	if err := json.Unmarshal(raw, &t); err != nil {
		return cachedToken{}, err
	}
	if t.AccessToken == "" {
		return cachedToken{}, errors.New("empty token cache")
	}
	return t, nil
}

func (c *Client) writeTokenCache(t cachedToken) error {
	if err := os.MkdirAll(filepath.Dir(c.TokenCachePath), 0o755); err != nil {
		return err
	}
	raw, _ := json.Marshal(t)
	return os.WriteFile(c.TokenCachePath, raw, 0o600)
}
```

`internal/kis/client.go`:

```go
// Package kis 는 한국투자증권 Open API 클라이언트다.
package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	rateLimitMsgCd = "EGW00201"
	maxRateRetries = 3
)

type Client struct {
	BaseURL        string
	AppKey         string
	AppSecret      string
	TokenCachePath string
	HTTP           *http.Client

	limiter   *limiter
	retryWait time.Duration
	now       func() time.Time

	mu  sync.Mutex
	tok *cachedToken
}

func New(baseURL, appKey, appSecret, tokenCachePath string, rps float64) *Client {
	return &Client{
		BaseURL:        baseURL,
		AppKey:         appKey,
		AppSecret:      appSecret,
		TokenCachePath: tokenCachePath,
		HTTP:           &http.Client{Timeout: 30 * time.Second},
		limiter:        newLimiter(rps),
		retryWait:      time.Second,
		now:            time.Now,
	}
}

type envelope struct {
	RtCd  string `json:"rt_cd"`
	MsgCd string `json:"msg_cd"`
	Msg1  string `json:"msg1"`
}

// get 은 GET 호출을 수행하고 out 에 응답 JSON 을 넣는다.
// 401 은 토큰을 한 번 재발급해 재시도, EGW00201 은 retryWait 후 최대 maxRateRetries 회 재시도한다.
func (c *Client) get(ctx context.Context, path, trID string, params url.Values, out any) error {
	refreshed := false
	rateRetries := 0
	for {
		tok, err := c.Token(ctx)
		if err != nil {
			return err
		}
		status, body, err := c.do(ctx, path, trID, params, tok)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized {
			if refreshed {
				return fmt.Errorf("kis: %s: 401 after token refresh: %s", path, body)
			}
			refreshed = true
			c.invalidateToken()
			continue
		}
		var env envelope
		_ = json.Unmarshal(body, &env)
		if env.MsgCd == rateLimitMsgCd {
			rateRetries++
			if rateRetries > maxRateRetries {
				return fmt.Errorf("kis: %s: rate limit exceeded after %d retries", path, maxRateRetries)
			}
			select {
			case <-time.After(c.retryWait):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		if status != http.StatusOK {
			return fmt.Errorf("kis: %s: HTTP %d: %s", path, status, body)
		}
		if env.RtCd != "0" {
			return fmt.Errorf("kis: %s: %s %s", path, env.MsgCd, env.Msg1)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("kis: %s: decode: %w", path, err)
		}
		return nil
	}
}

func (c *Client) do(ctx context.Context, path, trID string, params url.Values, tok string) (int, []byte, error) {
	if err := c.limiter.wait(ctx); err != nil {
		return 0, nil, err
	}
	u := c.BaseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	req.Header.Set("authorization", "Bearer "+tok)
	req.Header.Set("appkey", c.AppKey)
	req.Header.Set("appsecret", c.AppSecret)
	req.Header.Set("tr_id", trID)
	req.Header.Set("custtype", "P")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("kis: %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("kis: %s: read body: %w", path, err)
	}
	return resp.StatusCode, body, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/kis/ -v 2>&1 | tail -20`
Expected: 7개 테스트 PASS. `TestLimiterSpacesCalls`는 약 60ms 걸린다.

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/kis
git commit -m "feat(kis): 토큰 캐시, 호출 제한, 401·초당제한 재시도"
```

---

### Task 2: 종목 일봉 조회와 100건 분할

**Files:**
- Create: `internal/kis/chart.go`
- Test: `internal/kis/chart_test.go`

**Interfaces:**
- Consumes: Task 1의 `get`, `newFakeKIS`. 4단계의 `data.Bar`, `data.KST`, `data.ParseDate`.
- Produces: `(*Client).DailyBars(ctx, code string, from, to time.Time) ([]data.Bar, error)` — 오름차순, 중복 없음.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/kis/chart_test.go`:

```go
package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

// businessDays 는 [from, to] 의 평일을 오름차순으로 돌려준다.
func businessDays(from, to time.Time) []time.Time {
	var out []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			out = append(out, d)
		}
	}
	return out
}

// serveItemChart 는 요청 기간의 평일 봉을 최신순으로 최대 100건 돌려준다 (실서버와 같은 제한).
// blank 에 든 날짜는 가격이 빈 문자열인 행으로 돌려준다.
func serveItemChart(t *testing.T, f *fakeKIS, calls *[][2]string, blank map[string]bool) {
	t.Helper()
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("tr_id") != "FHKST03010100" || q.Get("FID_COND_MRKT_DIV_CODE") != "J" || q.Get("FID_PERIOD_DIV_CODE") != "D" ||
			q.Get("FID_ORG_ADJ_PRC") != "0" || q.Get("FID_INPUT_ISCD") != "005930" {
			http.Error(w, "bad params: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		d1, d2 := q.Get("FID_INPUT_DATE_1"), q.Get("FID_INPUT_DATE_2")
		*calls = append(*calls, [2]string{d1, d2})
		from, _ := time.ParseInLocation("20060102", d1, data.KST)
		to, _ := time.ParseInLocation("20060102", d2, data.KST)
		days := businessDays(from, to)
		var rows []map[string]string
		for i := len(days) - 1; i >= 0 && len(rows) < 100; i-- {
			ds := days[i].Format("20060102")
			row := map[string]string{"stck_bsop_date": ds, "stck_oprc": "100", "stck_hgpr": "110", "stck_lwpr": "90",
				"stck_clpr": fmt.Sprint(1000 + days[i].YearDay()), "acml_vol": "5000", "acml_tr_pbmn": "1", "flng_cls_code": "00", "prtt_rate": "0.00", "mod_yn": "N"}
			if blank[ds] {
				row["stck_oprc"], row["stck_hgpr"], row["stck_lwpr"], row["stck_clpr"], row["acml_vol"] = "", "", "", "", ""
			}
			rows = append(rows, row)
		}
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output1": map[string]string{}, "output2": rows})
	})
}

func TestDailyBarsChunksAndSorts(t *testing.T) {
	f := newFakeKIS(t)
	var calls [][2]string
	serveItemChart(t, f, &calls, map[string]bool{"20240215": true})
	from, to := data.Date(2024, 1, 2), data.Date(2024, 12, 31)
	bars, err := f.client.DailyBars(context.Background(), "005930", from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := businessDays(from, to)
	if len(bars) != len(want)-1 {
		t.Fatalf("got %d bars, want %d (one blank row skipped)", len(bars), len(want)-1)
	}
	for i := 1; i < len(bars); i++ {
		if !bars[i].Date.After(bars[i-1].Date) {
			t.Fatalf("not strictly ascending at %d: %v %v", i, bars[i-1].Date, bars[i].Date)
		}
	}
	if !bars[0].Date.Equal(from) || !bars[len(bars)-1].Date.Equal(data.Date(2024, 12, 31)) {
		t.Errorf("range: first=%v last=%v", bars[0].Date, bars[len(bars)-1].Date)
	}
	if bars[0].Close != 1002 || bars[0].Volume != 5000 || bars[0].High != 110 {
		t.Errorf("first bar = %+v", bars[0])
	}
	if len(calls) < 3 || calls[0][1] != "20241231" || calls[len(calls)-1][0] != "20240102" {
		t.Errorf("calls = %v", calls)
	}
	for _, c := range calls {
		a, _ := time.Parse("20060102", c[0])
		b, _ := time.Parse("20060102", c[1])
		if b.Sub(a) > 139*24*time.Hour {
			t.Errorf("chunk too wide: %v", c)
		}
	}
}

func TestDailyBarsStopsOnEmptyResponse(t *testing.T) {
	f := newFakeKIS(t)
	var calls [][2]string
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		calls = append(calls, [2]string{q.Get("FID_INPUT_DATE_1"), q.Get("FID_INPUT_DATE_2")})
		if q.Get("FID_INPUT_DATE_1") < "20240101" { // 첫 청크(20240416~)는 데이터, 둘째 청크(20231128~)는 빈 응답
			json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": []map[string]string{
			{"stck_bsop_date": "20240902", "stck_oprc": "1", "stck_hgpr": "2", "stck_lwpr": "1", "stck_clpr": "2", "acml_vol": "3"},
		}})
	})
	bars, err := f.client.DailyBars(context.Background(), "005930", data.Date(2020, 1, 1), data.Date(2024, 9, 2))
	if err != nil || len(bars) != 1 {
		t.Fatalf("bars=%v err=%v", bars, err)
	}
	if len(calls) != 2 {
		t.Errorf("expected stop after first empty chunk, calls=%v", calls)
	}
}

func TestDailyBarsFromAfterTo(t *testing.T) {
	f := newFakeKIS(t)
	bars, err := f.client.DailyBars(context.Background(), "005930", data.Date(2024, 9, 3), data.Date(2024, 9, 2))
	if err != nil || len(bars) != 0 {
		t.Errorf("expected no calls and no bars: %v %v", bars, err)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/kis/ -run DailyBars 2>&1 | head -5`
Expected: 컴파일 실패 (`DailyBars` 없음)

- [ ] **Step 3: 구현**

`internal/kis/chart.go`:

```go
package kis

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

const (
	itemChartPath = "/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice"
	itemChartTrID = "FHKST03010100"
	// 호출당 최대 100건. 달력일 140일 ≈ 영업일 96일이라 한 번에 다 들어온다.
	chunkCalendarDays = 140
	apiDateLayout     = "20060102"
)

type itemRow struct {
	Date   string `json:"stck_bsop_date"`
	Open   string `json:"stck_oprc"`
	High   string `json:"stck_hgpr"`
	Low    string `json:"stck_lwpr"`
	Close  string `json:"stck_clpr"`
	Volume string `json:"acml_vol"`
}

// DailyBars 는 [from, to] 의 수정주가 일봉을 오름차순으로 돌려준다.
// 기간을 종료일에서 거꾸로 140일씩 잘라 호출하고, 빈 응답을 받으면 멈춘다.
func (c *Client) DailyBars(ctx context.Context, code string, from, to time.Time) ([]data.Bar, error) {
	byDate := map[string]data.Bar{}
	err := c.walkChunks(from, to, func(start, end time.Time) (int, error) {
		var resp struct {
			Output2 []itemRow `json:"output2"`
		}
		params := url.Values{
			"FID_COND_MRKT_DIV_CODE": {"J"},
			"FID_INPUT_ISCD":         {code},
			"FID_INPUT_DATE_1":       {start.Format(apiDateLayout)},
			"FID_INPUT_DATE_2":       {end.Format(apiDateLayout)},
			"FID_PERIOD_DIV_CODE":    {"D"},
			"FID_ORG_ADJ_PRC":        {"0"},
		}
		if err := c.get(ctx, itemChartPath, itemChartTrID, params, &resp); err != nil {
			return 0, err
		}
		for _, r := range resp.Output2 {
			if r.Close == "" || r.Open == "" {
				continue // 거래정지 등으로 가격이 비어 있는 행
			}
			b, err := parseItemRow(r)
			if err != nil {
				return 0, fmt.Errorf("kis: %s %s: %w", code, r.Date, err)
			}
			byDate[r.Date] = b
		}
		return len(resp.Output2), nil
	})
	if err != nil {
		return nil, err
	}
	return sortBars(byDate), nil
}

// walkChunks 는 to 에서 from 쪽으로 chunkCalendarDays 단위로 call 을 호출한다. call 이 0건을 돌려주면 멈춘다.
func (c *Client) walkChunks(from, to time.Time, call func(start, end time.Time) (int, error)) error {
	end := to
	for !end.Before(from) {
		start := end.AddDate(0, 0, -(chunkCalendarDays - 1))
		if start.Before(from) {
			start = from
		}
		n, err := call(start, end)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		end = start.AddDate(0, 0, -1)
	}
	return nil
}

func parseItemRow(r itemRow) (data.Bar, error) {
	d, err := time.ParseInLocation(apiDateLayout, r.Date, data.KST)
	if err != nil {
		return data.Bar{}, err
	}
	vals := [5]int64{}
	for i, s := range []string{r.Open, r.High, r.Low, r.Close, r.Volume} {
		if vals[i], err = strconv.ParseInt(s, 10, 64); err != nil {
			return data.Bar{}, fmt.Errorf("field %d %q: %w", i, s, err)
		}
	}
	return data.Bar{Date: d, Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3], Volume: vals[4]}, nil
}

func sortBars(byDate map[string]data.Bar) []data.Bar {
	out := make([]data.Bar, 0, len(byDate))
	for _, b := range byDate {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/kis/ -run DailyBars -v 2>&1 | tail -8`
Expected: 3개 PASS

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/kis/chart.go internal/kis/chart_test.go
git commit -m "feat(kis): 종목 일봉 조회와 100건 분할"
```

---

### Task 3: 지수 일봉 조회

**Files:**
- Modify: `internal/kis/chart.go` (아래 코드를 파일 끝에 추가)
- Test: `internal/kis/chart_test.go` (아래 테스트 추가)

**Interfaces:**
- Produces: `(*Client).IndexBars(ctx, market string, from, to time.Time) ([]data.IndexBar, error)`. `market`은 `"kospi"`(업종코드 0001) 또는 `"kosdaq"`(1001).

- [ ] **Step 1: 실패하는 테스트 추가**

`internal/kis/chart_test.go` 끝에 추가:

```go
func TestIndexBars(t *testing.T) {
	f := newFakeKIS(t)
	var gotCode string
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("tr_id") != "FHKUP03500100" || q.Get("FID_COND_MRKT_DIV_CODE") != "U" || q.Get("FID_PERIOD_DIV_CODE") != "D" {
			http.Error(w, "bad params: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		gotCode = q.Get("FID_INPUT_ISCD")
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": []map[string]string{
			{"stck_bsop_date": "20240903", "bstp_nmix_prpr": "2664.63", "bstp_nmix_oprc": "2680.10", "bstp_nmix_hgpr": "2690.55", "bstp_nmix_lwpr": "2660.01", "acml_vol": "1"},
			{"stck_bsop_date": "20240902", "bstp_nmix_prpr": "2681.00", "bstp_nmix_oprc": "2670.00", "bstp_nmix_hgpr": "2685.00", "bstp_nmix_lwpr": "2665.00", "acml_vol": "1"},
		}})
	})
	bars, err := f.client.IndexBars(context.Background(), "kosdaq", data.Date(2024, 9, 1), data.Date(2024, 9, 3))
	if err != nil {
		t.Fatal(err)
	}
	if gotCode != "1001" {
		t.Errorf("kosdaq index code = %q, want 1001", gotCode)
	}
	if len(bars) != 2 || !bars[0].Date.Equal(data.Date(2024, 9, 2)) || bars[1].Close != 2664.63 || bars[1].Open != 2680.10 {
		t.Errorf("bars = %+v", bars)
	}
	if _, err := f.client.IndexBars(context.Background(), "nyse", data.Date(2024, 9, 1), data.Date(2024, 9, 3)); err == nil {
		t.Error("unknown market should error before calling")
	}
	f.client.IndexBars(context.Background(), "kospi", data.Date(2024, 9, 1), data.Date(2024, 9, 3))
	if gotCode != "0001" {
		t.Errorf("kospi index code = %q, want 0001", gotCode)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/kis/ -run IndexBars 2>&1 | head -5`
Expected: 컴파일 실패

- [ ] **Step 3: 구현 (chart.go 끝에 추가)**

```go
const (
	indexChartPath = "/uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice"
	indexChartTrID = "FHKUP03500100"
)

var indexCodes = map[string]string{"kospi": "0001", "kosdaq": "1001"}

type indexRow struct {
	Date  string `json:"stck_bsop_date"`
	Open  string `json:"bstp_nmix_oprc"`
	High  string `json:"bstp_nmix_hgpr"`
	Low   string `json:"bstp_nmix_lwpr"`
	Close string `json:"bstp_nmix_prpr"`
}

// IndexBars 는 시장 종합지수(코스피 0001, 코스닥 1001)의 일봉을 오름차순으로 돌려준다.
func (c *Client) IndexBars(ctx context.Context, market string, from, to time.Time) ([]data.IndexBar, error) {
	code, ok := indexCodes[market]
	if !ok {
		return nil, fmt.Errorf("kis: unknown market %q", market)
	}
	byDate := map[string]data.IndexBar{}
	err := c.walkChunks(from, to, func(start, end time.Time) (int, error) {
		var resp struct {
			Output2 []indexRow `json:"output2"`
		}
		params := url.Values{
			"FID_COND_MRKT_DIV_CODE": {"U"},
			"FID_INPUT_ISCD":         {code},
			"FID_INPUT_DATE_1":       {start.Format(apiDateLayout)},
			"FID_INPUT_DATE_2":       {end.Format(apiDateLayout)},
			"FID_PERIOD_DIV_CODE":    {"D"},
		}
		if err := c.get(ctx, indexChartPath, indexChartTrID, params, &resp); err != nil {
			return 0, err
		}
		for _, r := range resp.Output2 {
			if r.Close == "" {
				continue
			}
			b, err := parseIndexRow(r)
			if err != nil {
				return 0, fmt.Errorf("kis: index %s %s: %w", market, r.Date, err)
			}
			byDate[r.Date] = b
		}
		return len(resp.Output2), nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]data.IndexBar, 0, len(byDate))
	for _, b := range byDate {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

func parseIndexRow(r indexRow) (data.IndexBar, error) {
	d, err := time.ParseInLocation(apiDateLayout, r.Date, data.KST)
	if err != nil {
		return data.IndexBar{}, err
	}
	vals := [4]float64{}
	for i, s := range []string{r.Open, r.High, r.Low, r.Close} {
		if vals[i], err = strconv.ParseFloat(s, 64); err != nil {
			return data.IndexBar{}, fmt.Errorf("field %d %q: %w", i, s, err)
		}
	}
	return data.IndexBar{Date: d, Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3]}, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/kis/ -v 2>&1 | tail -15`
Expected: 전부 PASS (11개)

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/kis/chart.go internal/kis/chart_test.go
git commit -m "feat(kis): 코스피·코스닥 지수 일봉 조회"
```

---


## 완료 기준

- `go test ./internal/kis/...` 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- 다음 단계(6단계 유니버스·수집)는 `kis.New`, `(*Client).DailyBars`, `(*Client).IndexBars`, `newFakeKIS`를 그대로 소비한다
