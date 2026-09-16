# 2단계: LS증권 실시간 뉴스·지수 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** TUI 상단 뉴스 줄과 하단 코스피·코스닥 지수 줄이 LS증권 웹소켓에서 받은 실데이터로 움직이고, 연결이 끊기면 `미연결`로 돌아갔다가 자동 재연결된다.

**Architecture:** `ls`(토큰 REST, 웹소켓 구독·파싱·재연결. `chan Event`로 내보내고 TUI를 모른다) → `tui/run.go`가 이벤트를 `tea.Msg`로 바꿔 `p.Send`. `fake.go`에서 뉴스·지수 항목을 지운다. 로그는 3단계 전이라 `slog.DiscardHandler`로 버리고, 디버그용 `trader ls-probe` 서브커맨드가 stderr 로 이벤트를 찍는다.

**Tech Stack:** Go 1.27, `github.com/coder/websocket`(구 nhooyr.io/websocket), 표준 `net/http`·`log/slog`. 테스트는 `httptest` + `websocket.Accept`로 가짜 서버.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (3절 의존성, 5절 `ls` 설정, 10.1절 상단·하단, 10.2절 LS 클라이언트, 10.4절 테스트, 11절 미연결 처리)

**선행:** 1단계 완료 (`internal/config`, `internal/tui` 존재, 18 테스트 통과). `.env`에 `LS_APP_KEY`, `LS_APP_SECRET` 있음.

## Global Constraints

- 모듈 경로: `github.com/gong-yeongbin/my-trading`
- 이 단계에서 추가하는 외부 의존성은 `github.com/coder/websocket` 하나뿐. 테스트 프레임워크 추가 금지.
- LS 사양(스펙 10.2절): 토큰 `POST {base_url}/oauth2/token` form-urlencoded `appkey`,`appsecretkey`,`grant_type=client_credentials`,`scope=oob` → `access_token`,`expires_in`. 구독 `{"header":{"token","tr_type":"3"},"body":{"tr_cd","tr_key"}}`. 수신 `{"header":{"tr_cd","tr_key"},"body":{…}}`. 뉴스 `NWS`/`NWS001` (`title`). 지수 `IJ_`/`001`(코스피)·`301`(코스닥) (`upcode`,`jisu`,`change`,`drate`,`sign`). `sign` 4·5 면 하락.
- 재연결: 5초부터 2배씩 최대 60초. 연결 성공 후 끊기면 5초부터 다시. 재연결 후 구독 재전송. 20초마다 ping.
- `ls` 패키지는 `tui`·`config`를 import 하지 않는다. 자기 `Config` 구조체를 받는다.
- TUI 는 `Disconnected` 를 받으면 뉴스·지수를 `미연결`로 되돌린다. LS 앱키가 없으면 클라이언트를 만들지 않는다.
- LS 실서버는 테스트에서 호출하지 않는다. 마지막 수동 확인에서만.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/*.json`은 읽기 금지.
- 파일 편집 후 `gofmt -l .` 결과가 비어 있어야 한다.
- 실제 터미널 렌더링은 Claude가 볼 수 없다. 수동 확인은 사용자가 실행하고 결과를 말로 전달한다.

---

## 파일 구조

```
internal/ls/client.go         Config, Client, New, 이벤트 타입
internal/ls/token.go          Token: 발급·캐시
internal/ls/ws.go             Run: 연결·구독·ping·재연결 루프
internal/ls/parse.go          parseMessage: 수신 JSON → Event
internal/ls/token_test.go
internal/ls/ws_test.go        가짜 토큰·웹소켓 서버
internal/ls/parse_test.go
internal/tui/types.go         DisconnectedMsg 추가 (수정)
internal/tui/model.go         DisconnectedMsg 처리 (수정)
internal/tui/view.go          출처 없는 뉴스 표시 (수정)
internal/tui/ls.go            ls.Event → tea.Msg 변환
internal/tui/ls_test.go
internal/tui/run.go           LS 클라이언트 배선 (수정)
internal/tui/fake.go          뉴스·지수 항목 삭제 (수정)
internal/tui/model_test.go    DisconnectedMsg 테스트 추가 (수정)
internal/tui/view_test.go     출처 없는 뉴스 테스트 추가 (수정)
cmd/trader/main.go            ls-probe 서브커맨드 추가 (수정)
```

---

### Task 1: LS 토큰 클라이언트

**Files:**
- Create: `internal/ls/client.go`, `internal/ls/token.go`
- Test: `internal/ls/token_test.go`

**Interfaces:**
- Produces: `ls.Config{BaseURL, WSURL, AppKey, AppSecret, TokenCache string}`, `ls.New(cfg Config, logger *slog.Logger) *Client`, `(*Client).Token(ctx) (string, error)`, 이벤트 타입 `ls.Event`(인터페이스), `ls.News`, `ls.Index`, `ls.Connected`, `ls.Disconnected`. 비공개 `requestCount`용 훅은 없다 — 테스트는 서버 쪽에서 요청 수를 센다. Task 2 가 `Token`과 `Client.http`, `Client.log`, `Client.retryMin/retryMax`를 쓴다.

- [ ] **Step 1: 의존성 추가**

```bash
cd /Users/gong-yeongbin/orca/projects/my-trading
go get github.com/coder/websocket@latest
```

Expected: `go.mod`에 `github.com/coder/websocket v1.8.x` require (이 Task 에서는 아직 import 하지 않으므로 `// indirect`. Task 2 에서 직접 의존성이 됨).

- [ ] **Step 2: 타입과 생성자 작성**

`internal/ls/client.go`:

```go
// Package ls 는 LS증권 Open API 의 실시간 웹소켓 클라이언트다. 뉴스 제목과 업종 지수를 받아 Event 채널로 내보낸다.
// 이 패키지는 TUI 와 config 를 모른다.
package ls

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	BaseURL    string // https://openapi.ls-sec.co.kr:8080
	WSURL      string // wss://openapi.ls-sec.co.kr:9443/websocket
	AppKey     string
	AppSecret  string
	TokenCache string // 토큰 캐시 파일 경로
}

type Client struct {
	cfg  Config
	http *http.Client
	log  *slog.Logger

	mu        sync.Mutex
	token     string
	expiresAt time.Time

	retryMin, retryMax time.Duration // 재연결 대기. 테스트에서 줄인다.
}

func New(cfg Config, logger *slog.Logger) *Client {
	return &Client{
		cfg:      cfg,
		http:     &http.Client{Timeout: 10 * time.Second},
		log:      logger,
		retryMin: 5 * time.Second,
		retryMax: 60 * time.Second,
	}
}

// Event 는 Run 이 채널로 내보내는 값. News, Index, Connected, Disconnected 중 하나.
type Event interface{ isEvent() }

// News 는 NWS 실시간 뉴스 제목 한 건. 출처 필드는 API 에 없다.
type News struct {
	Date, Time, ID, Title, Code string
}

// Index 는 IJ_ 업종지수 한 건. Code 는 업종코드(001 코스피, 301 코스닥). ChangePct 는 0.008 = +0.8%.
type Index struct {
	Code      string
	Value     float64
	Change    float64
	ChangePct float64
	Time      string
}

// Connected 는 웹소켓 연결·구독이 끝났을 때, Disconnected 는 연결이 끊겼을 때 한 번 보낸다.
type Connected struct{}
type Disconnected struct{ Err error }

func (News) isEvent()         {}
func (Index) isEvent()        {}
func (Connected) isEvent()    {}
func (Disconnected) isEvent() {}
```

- [ ] **Step 3: 실패하는 테스트 작성**

`internal/ls/token_test.go`:

```go
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
```

- [ ] **Step 4: 테스트 실패 확인**

Run: `go test ./internal/ls/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: cachedToken`, `c.Token`)

- [ ] **Step 5: 구현**

`internal/ls/token.go`:

```go
package ls

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tokenSafety = 30 * time.Second

// cachedToken 은 token_cache 파일 형식.
type cachedToken struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Token 은 유효한 접근 토큰을 돌려준다. 메모리 → 파일 캐시 → 발급 순서.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" {
		c.loadCache()
	}
	if c.token != "" && time.Until(c.expiresAt) > tokenSafety {
		return c.token, nil
	}
	tok, exp, err := c.requestToken(ctx)
	if err != nil {
		return "", err
	}
	c.token, c.expiresAt = tok, exp
	c.saveCache()
	return tok, nil
}

func (c *Client) requestToken(ctx context.Context) (string, time.Time, error) {
	form := url.Values{
		"appkey":       {c.cfg.AppKey},
		"appsecretkey": {c.cfg.AppSecret},
		"grant_type":   {"client_credentials"},
		"scope":        {"oob"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("ls token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("ls token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", time.Time{}, fmt.Errorf("ls token: parse: %w", err)
	}
	if out.AccessToken == "" {
		return "", time.Time{}, errors.New("ls token: access_token 없음")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 86400
	}
	return out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn) * time.Second), nil
}

func (c *Client) loadCache() {
	b, err := os.ReadFile(c.cfg.TokenCache)
	if err != nil {
		return
	}
	var ct cachedToken
	if json.Unmarshal(b, &ct) == nil && ct.AccessToken != "" {
		c.token, c.expiresAt = ct.AccessToken, ct.ExpiresAt
	}
}

func (c *Client) saveCache() {
	b, _ := json.Marshal(cachedToken{AccessToken: c.token, ExpiresAt: c.expiresAt})
	if err := os.MkdirAll(filepath.Dir(c.cfg.TokenCache), 0o700); err != nil {
		c.log.Warn("ls token cache dir", "err", err)
		return
	}
	if err := os.WriteFile(c.cfg.TokenCache, b, 0o600); err != nil {
		c.log.Warn("ls token cache write", "err", err)
	}
}
```

- [ ] **Step 6: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./internal/ls/ && go test ./internal/ls/ -v 2>&1 | tail -12`
Expected: 4개 테스트 PASS, gofmt 출력 없음

- [ ] **Step 7: 커밋 요청**

```bash
git add go.mod go.sum internal/ls
git commit -m "feat(ls): LS증권 토큰 발급·캐시"
```

---

### Task 2: 웹소켓 구독·파싱·재연결

**Files:**
- Create: `internal/ls/parse.go`, `internal/ls/ws.go`
- Test: `internal/ls/parse_test.go`, `internal/ls/ws_test.go`

**Interfaces:**
- Consumes: Task 1 의 `Client`(`cfg`, `http`, `log`, `retryMin`, `retryMax`), `Token`, 이벤트 타입.
- Produces: `ls.Subscription{TrCd, TrKey string}`, `(*Client).Run(ctx context.Context, subs []Subscription, events chan<- Event)` — ctx 가 끝날 때까지 블록. 비공개 `parseMessage([]byte) (Event, bool)`.

- [ ] **Step 1: 파싱 테스트 작성**

`internal/ls/parse_test.go`:

```go
package ls

import "testing"

func TestParseNews(t *testing.T) {
	raw := []byte(`{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"date":"20260916","time":"142800","id":"01234567","title":"삼성전자, 3분기 파운드리 수주 확대 전망","code":"005930","realkey":"","bodysize":"1234"}}`)
	ev, ok := parseMessage(raw)
	if !ok {
		t.Fatal("expected event")
	}
	n, isNews := ev.(News)
	if !isNews {
		t.Fatalf("event type = %T", ev)
	}
	if n.Title != "삼성전자, 3분기 파운드리 수주 확대 전망" || n.Date != "20260916" || n.Time != "142800" || n.ID != "01234567" || n.Code != "005930" {
		t.Errorf("news = %+v", n)
	}
}

func TestParseIndex(t *testing.T) {
	cases := []struct {
		name    string
		sign    string
		jisu    string
		drate   string
		change  string
		wantPct float64
		wantChg float64
	}{
		{"up", "2", "2712.40", "0.80", "21.50", 0.008, 21.5},
		{"down", "5", "782.15", "0.40", "3.10", -0.004, -3.1},
		{"lower-limit", "4", "700.00", "10.00", "70.00", -0.10, -70},
		{"flat", "3", "1000.00", "0.00", "0.00", 0, 0},
		{"numeric-json", "2", "", "", "", 0.008, 21.5}, // body 값이 숫자로 와도 처리
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw string
			if tc.name == "numeric-json" {
				raw = `{"header":{"tr_cd":"IJ_","tr_key":"001"},"body":{"upcode":"001","jisu":2712.40,"drate":0.80,"change":21.50,"sign":"2","time":"143210"}}`
			} else {
				raw = `{"header":{"tr_cd":"IJ_","tr_key":"301"},"body":{"upcode":"301","jisu":"` + tc.jisu + `","drate":"` + tc.drate + `","change":"` + tc.change + `","sign":"` + tc.sign + `","time":"143210"}}`
			}
			ev, ok := parseMessage([]byte(raw))
			if !ok {
				t.Fatal("expected event")
			}
			ix, isIndex := ev.(Index)
			if !isIndex {
				t.Fatalf("event type = %T", ev)
			}
			if tc.name != "numeric-json" && ix.Code != "301" {
				t.Errorf("code = %q", ix.Code)
			}
			if !near(ix.ChangePct, tc.wantPct) || !near(ix.Change, tc.wantChg) {
				t.Errorf("pct = %v want %v, change = %v want %v", ix.ChangePct, tc.wantPct, ix.Change, tc.wantChg)
			}
			if ix.Time != "143210" {
				t.Errorf("time = %q", ix.Time)
			}
		})
	}
}

func TestParseIgnoresOthers(t *testing.T) {
	for _, raw := range []string{
		`{"header":{"tr_cd":"NWS","rsp_cd":"00000","rsp_msg":"정상처리"}}`, // 구독 응답 (body 없음)
		`{"header":{"tr_cd":"IJ_","tr_key":"001"},"body":{"upcode":"001","jisu":"abc","sign":"2"}}`, // 숫자 아님
		`{"header":{"tr_cd":"S3_","tr_key":"005930"},"body":{"price":"71200"}}`, // 모르는 TR
		`not json`,
	} {
		if ev, ok := parseMessage([]byte(raw)); ok {
			t.Errorf("expected no event for %s, got %+v", raw, ev)
		}
	}
}

func near(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
```

- [ ] **Step 2: 파싱 구현**

`internal/ls/parse.go`:

```go
package ls

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type wsMessage struct {
	Header struct {
		TrCd  string `json:"tr_cd"`
		TrKey string `json:"tr_key"`
	} `json:"header"`
	Body map[string]any `json:"body"`
}

// parseMessage 는 수신 JSON 을 Event 로 바꾼다. 구독 응답, 모르는 TR, 깨진 숫자는 (nil, false).
func parseMessage(data []byte) (Event, bool) {
	var m wsMessage
	if err := json.Unmarshal(data, &m); err != nil || m.Body == nil {
		return nil, false
	}
	switch m.Header.TrCd {
	case "NWS":
		return News{
			Date:  field(m.Body, "date"),
			Time:  field(m.Body, "time"),
			ID:    field(m.Body, "id"),
			Title: field(m.Body, "title"),
			Code:  field(m.Body, "code"),
		}, true
	case "IJ_":
		value, err1 := strconv.ParseFloat(field(m.Body, "jisu"), 64)
		change, err2 := strconv.ParseFloat(field(m.Body, "change"), 64)
		pct, err3 := strconv.ParseFloat(field(m.Body, "drate"), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, false
		}
		if s := field(m.Body, "sign"); s == "4" || s == "5" {
			change, pct = -abs(change), -abs(pct)
		} else {
			change, pct = abs(change), abs(pct)
		}
		return Index{
			Code:      field(m.Body, "upcode"),
			Value:     value,
			Change:    change,
			ChangePct: pct / 100,
			Time:      field(m.Body, "time"),
		}, true
	}
	return nil, false
}

// field 는 body 값을 문자열로 꺼낸다. LS 는 문자열로 보내지만 숫자로 와도 처리한다.
func field(body map[string]any, key string) string {
	switch v := body[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
```

- [ ] **Step 3: 파싱 테스트 통과 확인**

Run: `go test ./internal/ls/ -run TestParse -v 2>&1 | tail -12`
Expected: TestParseNews, TestParseIndex(5 서브테스트), TestParseIgnoresOthers PASS

- [ ] **Step 4: 웹소켓 테스트 작성**

`internal/ls/ws_test.go`:

```go
package ls

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// subscribeMsg 는 클라이언트가 보내는 구독 메시지 (테스트에서 디코딩용).
type subscribeMsg struct {
	Header struct {
		Token  string `json:"token"`
		TrType string `json:"tr_type"`
	} `json:"header"`
	Body struct {
		TrCd  string `json:"tr_cd"`
		TrKey string `json:"tr_key"`
	} `json:"body"`
}

// wsServer 는 토큰 엔드포인트와 웹소켓을 함께 제공하는 가짜 서버.
// onConn 은 연결마다 호출되며 구독 메시지 nsubs 개를 읽은 뒤의 동작을 정의한다.
type wsServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	subs     [][]subscribeMsg // 연결별 받은 구독
	accepts  int
	onConn   func(conn *websocket.Conn, n int)
	nsubs    int
}

func newWSServer(t *testing.T, nsubs int, onConn func(conn *websocket.Conn, n int)) *wsServer {
	t.Helper()
	s := &wsServer{onConn: onConn, nsubs: nsubs}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	// 핸들러는 서버 고루틴에서 돌고 테스트 종료 뒤에도 살아 있을 수 있으므로 t 를 호출하지 않는다. 검증은 subs 기록으로 한다.
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		s.mu.Lock()
		s.accepts++
		n := s.accepts
		s.subs = append(s.subs, nil)
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := 0; i < s.nsubs; i++ {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var sm subscribeMsg
			if err := json.Unmarshal(data, &sm); err != nil {
				return
			}
			s.mu.Lock()
			s.subs[n-1] = append(s.subs[n-1], sm)
			s.mu.Unlock()
		}
		s.onConn(conn, n)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *wsServer) client(t *testing.T) *Client {
	t.Helper()
	c := New(Config{
		BaseURL: s.srv.URL, WSURL: strings.Replace(s.srv.URL, "http://", "ws://", 1) + "/websocket",
		AppKey: "k", AppSecret: "s", TokenCache: filepath.Join(t.TempDir(), "t.json"),
	}, slog.New(slog.DiscardHandler))
	c.retryMin, c.retryMax = 20*time.Millisecond, 50*time.Millisecond
	return c
}

var testSubs = []Subscription{{"NWS", "NWS001"}, {"IJ_", "001"}, {"IJ_", "301"}}

// send 는 서버 → 클라이언트 전송. 클라이언트가 먼저 끊었으면 오류를 무시한다 (테스트 종료 후 실행될 수 있음).
func send(conn *websocket.Conn, raw string) {
	_ = conn.Write(context.Background(), websocket.MessageText, []byte(raw))
}

// next 는 events 에서 다음 이벤트를 꺼낸다. 2초 안에 없으면 실패.
func next(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event within 2s")
		return nil
	}
}

func TestSubscribeAndReceive(t *testing.T) {
	s := newWSServer(t, len(testSubs), func(conn *websocket.Conn, n int) {
		send(conn, `{"header":{"tr_cd":"NWS","rsp_cd":"00000"}}`)
		send(conn, `{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"title":"제목1","date":"20260916","time":"1","id":"a","code":""}}`)
		send(conn, `{"header":{"tr_cd":"IJ_","tr_key":"301"},"body":{"upcode":"301","jisu":"782.15","drate":"0.40","change":"3.10","sign":"5","time":"143210"}}`)
		time.Sleep(200 * time.Millisecond) // 클라이언트가 읽을 시간
		conn.Close(websocket.StatusNormalClosure, "bye")
	})
	c := s.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 16)
	done := make(chan struct{})
	go func() { c.Run(ctx, testSubs, events); close(done) }()

	if _, ok := next(t, events).(Connected); !ok {
		t.Fatal("first event should be Connected")
	}
	n, ok := next(t, events).(News)
	if !ok || n.Title != "제목1" {
		t.Fatalf("expected News, got %+v", n)
	}
	ix, ok := next(t, events).(Index)
	if !ok || ix.Code != "301" || !near(ix.ChangePct, -0.004) {
		t.Fatalf("expected Index 301 -0.4%%, got %+v", ix)
	}
	if _, ok := next(t, events).(Disconnected); !ok {
		t.Fatal("expected Disconnected after server close")
	}

	s.mu.Lock()
	got := s.subs[0]
	s.mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("subscriptions received = %d", len(got))
	}
	for i, sm := range got {
		if sm.Header.Token != "tok" || sm.Header.TrType != "3" {
			t.Errorf("sub %d header = %+v", i, sm.Header)
		}
		if sm.Body.TrCd != testSubs[i].TrCd || sm.Body.TrKey != testSubs[i].TrKey {
			t.Errorf("sub %d body = %+v, want %+v", i, sm.Body, testSubs[i])
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestReconnectResubscribes(t *testing.T) {
	s := newWSServer(t, len(testSubs), func(conn *websocket.Conn, n int) {
		if n == 1 {
			conn.Close(websocket.StatusNormalClosure, "first drop")
			return
		}
		send(conn, `{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"title":"재연결 후"}}`)
		time.Sleep(200 * time.Millisecond)
		conn.Close(websocket.StatusNormalClosure, "")
	})
	c := s.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 16)
	go c.Run(ctx, testSubs, events)

	var titles []string
	var connects, disconnects int
	deadline := time.After(3 * time.Second)
	for len(titles) == 0 {
		select {
		case ev := <-events:
			switch e := ev.(type) {
			case Connected:
				connects++
			case Disconnected:
				disconnects++
			case News:
				titles = append(titles, e.Title)
			}
		case <-deadline:
			t.Fatalf("no news after reconnect (connects=%d disconnects=%d)", connects, disconnects)
		}
	}
	if connects < 2 || disconnects < 1 {
		t.Errorf("connects=%d disconnects=%d, want >=2 and >=1", connects, disconnects)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subs) < 2 || len(s.subs[1]) != 3 {
		t.Errorf("second connection should receive 3 subscriptions, got %v", s.subs)
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	c := &Client{retryMin: 5 * time.Second, retryMax: 60 * time.Second}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
	d := c.retryMin
	for i, w := range want {
		if d != w {
			t.Errorf("step %d = %v, want %v", i, d, w)
		}
		d = c.nextDelay(d)
	}
}

func TestRunStopsOnCancelWhileWaiting(t *testing.T) {
	// 토큰 서버가 없으므로 연결 실패 → 대기 중 취소.
	c := New(Config{BaseURL: "http://127.0.0.1:1", WSURL: "ws://127.0.0.1:1/websocket", AppKey: "k", AppSecret: "s", TokenCache: filepath.Join(t.TempDir(), "t.json")}, slog.New(slog.DiscardHandler))
	c.retryMin, c.retryMax = time.Hour, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 4)
	done := make(chan struct{})
	go func() { c.Run(ctx, testSubs, events); close(done) }()
	if _, ok := next(t, events).(Disconnected); !ok {
		t.Fatal("expected Disconnected on connect failure")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel during backoff wait")
	}
}
```

- [ ] **Step 5: 웹소켓 테스트 실패 확인**

Run: `go test ./internal/ls/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: Subscription`, `c.Run`, `c.nextDelay`)

- [ ] **Step 6: 웹소켓 구현**

`internal/ls/ws.go`:

```go
package ls

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

const pingInterval = 20 * time.Second

type Subscription struct {
	TrCd, TrKey string
}

// Run 은 ctx 가 끝날 때까지 웹소켓 연결을 유지한다. 연결·구독이 끝나면 Connected, 끊기면 Disconnected 를 보내고
// retryMin 부터 2배씩 retryMax 까지 기다렸다가 다시 연결한다. 연결에 성공했던 뒤에는 대기를 retryMin 으로 되돌린다.
func (c *Client) Run(ctx context.Context, subs []Subscription, events chan<- Event) {
	delay := c.retryMin
	for {
		connected, err := c.session(ctx, subs, events)
		if ctx.Err() != nil {
			return
		}
		c.log.Warn("ls websocket 끊김", "err", err, "retry_in", delay)
		emit(ctx, events, Disconnected{Err: err})
		if !sleepCtx(ctx, delay) {
			return
		}
		if connected {
			delay = c.retryMin
		} else {
			delay = c.nextDelay(delay)
		}
	}
}

func (c *Client) nextDelay(d time.Duration) time.Duration {
	return min(d*2, c.retryMax)
}

// session 은 한 번의 연결을 수행한다. connected 는 구독까지 끝났는지.
func (c *Client) session(ctx context.Context, subs []Subscription, events chan<- Event) (connected bool, err error) {
	token, err := c.Token(ctx)
	if err != nil {
		return false, err
	}
	conn, _, err := websocket.Dial(ctx, c.cfg.WSURL, nil)
	if err != nil {
		return false, err
	}
	defer conn.CloseNow()

	for _, s := range subs {
		msg := map[string]any{
			"header": map[string]string{"token": token, "tr_type": "3"},
			"body":   map[string]string{"tr_cd": s.TrCd, "tr_key": s.TrKey},
		}
		b, _ := json.Marshal(msg)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			return false, err
		}
	}
	c.log.Info("ls websocket 연결", "subs", len(subs))
	emit(ctx, events, Connected{})

	// ping 이 실패하면 연결을 닫아 아래 Read 가 오류로 빠져나오게 한다.
	pctx, cancelPing := context.WithCancel(ctx)
	defer cancelPing()
	go func() {
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-t.C:
				if err := conn.Ping(pctx); err != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return true, err
		}
		if ev, ok := parseMessage(data); ok {
			emit(ctx, events, ev)
		}
	}
}

func emit(ctx context.Context, events chan<- Event, ev Event) {
	select {
	case events <- ev:
	case <-ctx.Done():
	}
}

// sleepCtx 는 d 만큼 기다린다. ctx 가 먼저 끝나면 false.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
```

- [ ] **Step 7: 전체 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./internal/ls/ && go test ./internal/ls/ -v 2>&1 | tail -25`
Expected: Task 1 의 4개 + 파싱 3개 + 웹소켓 4개 = 11개 테스트 PASS (서브테스트 별도). `go.mod`에서 `coder/websocket`이 직접 의존성이 됨 (`go mod tidy` 필요하면 실행).

- [ ] **Step 8: 커밋 요청**

```bash
git add go.mod go.sum internal/ls
git commit -m "feat(ls): 웹소켓 구독·파싱·재연결"
```

---

### Task 3: TUI 배선 — DisconnectedMsg, 이벤트 변환, Run 연결

**Files:**
- Modify: `internal/tui/types.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/run.go`, `internal/tui/fake.go`
- Create: `internal/tui/ls.go`
- Test: `internal/tui/ls_test.go` (new), `internal/tui/model_test.go`, `internal/tui/view_test.go` (추가)

**Interfaces:**
- Consumes: `ls.New`, `ls.Config`, `(*ls.Client).Run`, `ls.Subscription`, `ls.Event`/`News`/`Index`/`Connected`/`Disconnected`; `config.LSConfig`(`BaseURL, WSURL, AppKey, AppSecret, TokenCache`, `HasAppKey()`).
- Produces: `tui.DisconnectedMsg`, 비공개 `lsSubscriptions`, `lsToMsg(ls.Event) tea.Msg`. Task 4 의 `ls-probe` 가 `lsSubscriptions` 대신 자기 목록을 쓴다 (패키지 밖이므로). 3단계는 `run.go`의 `slog.New(slog.DiscardHandler)` 자리를 파일 로거로 바꾼다.

- [ ] **Step 1: 실패하는 테스트 추가**

`internal/tui/model_test.go` 끝에 추가:

```go
func TestDisconnectedResetsNewsAndIndex(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Title: "제목"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	m = send(m, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004})
	m = send(m, DisconnectedMsg{})
	if m.newsOK || m.kospi.Connected || m.kosdaq.Connected {
		t.Errorf("DisconnectedMsg should reset: newsOK=%v kospi=%v kosdaq=%v", m.newsOK, m.kospi.Connected, m.kosdaq.Connected)
	}
	if strings.Count(m.View(), "미연결") < 3 {
		t.Errorf("view should show 미연결 for news and both indexes:\n%s", m.View())
	}
}
```
(`model_test.go` import 에 `"strings"` 추가.)

`internal/tui/view_test.go` 끝에 추가:

```go
func TestViewNewsWithoutSource(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Title: "출처 없는 제목"})
	v := m.View()
	if !strings.Contains(v, " 출처 없는 제목") || strings.Contains(v, "[] 출처") {
		t.Errorf("news without source should render title only:\n%s", v)
	}
	m = send(m, NewsMsg{Source: "연합뉴스", Title: "출처 있는 제목"})
	if !strings.Contains(m.View(), "[연합뉴스] 출처 있는 제목") {
		t.Errorf("news with source should render [source] title:\n%s", m.View())
	}
}
```

`internal/tui/ls_test.go` (새 파일):

```go
package tui

import (
	"errors"
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/ls"
)

func TestLSToMsg(t *testing.T) {
	cases := []struct {
		name string
		ev   ls.Event
		want any
	}{
		{"news", ls.News{Title: "제목"}, NewsMsg{Title: "제목"}},
		{"kospi", ls.Index{Code: "001", Value: 2712.4, ChangePct: 0.008}, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008}},
		{"kosdaq", ls.Index{Code: "301", Value: 782.15, ChangePct: -0.004}, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004}},
		{"unknown-index", ls.Index{Code: "999"}, nil},
		{"connected", ls.Connected{}, nil},
		{"disconnected", ls.Disconnected{Err: errors.New("x")}, DisconnectedMsg{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lsToMsg(tc.ev)
			if got != tc.want {
				t.Errorf("lsToMsg(%+v) = %#v, want %#v", tc.ev, got, tc.want)
			}
		})
	}
}

func TestLSSubscriptions(t *testing.T) {
	want := []ls.Subscription{{TrCd: "NWS", TrKey: "NWS001"}, {TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"}}
	if len(lsSubscriptions) != len(want) {
		t.Fatalf("subs = %v", lsSubscriptions)
	}
	for i := range want {
		if lsSubscriptions[i] != want[i] {
			t.Errorf("sub %d = %v, want %v", i, lsSubscriptions[i], want[i])
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: DisconnectedMsg`, `lsToMsg`, `lsSubscriptions`)

- [ ] **Step 3: types.go 에 메시지 추가**

`internal/tui/types.go` 의 `LogMsg` 정의 뒤에 추가:

```go
// DisconnectedMsg 는 실시간 연결(LS)이 끊겼을 때. 뉴스·지수를 미연결로 되돌린다.
type DisconnectedMsg struct{}
```

- [ ] **Step 4: model.go 에 처리 추가**

`internal/tui/model.go` `Update` 의 `case LogMsg:` 블록 뒤에 추가:

```go
	case DisconnectedMsg:
		m.newsOK = false
		m.kospi.Connected = false
		m.kosdaq.Connected = false
```

- [ ] **Step 5: view.go 출처 없는 뉴스**

`internal/tui/view.go` 의 `headerLeft` 를 이렇게 바꾼다:

```go
func (m Model) headerLeft() string {
	if !m.newsOK {
		return " 미연결"
	}
	if m.news.Source == "" {
		return " " + m.news.Title
	}
	return fmt.Sprintf(" [%s] %s", m.news.Source, m.news.Title)
}
```

- [ ] **Step 6: 이벤트 변환 파일**

`internal/tui/ls.go`:

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/ls"
)

// lsSubscriptions 는 TUI 가 구독하는 LS 실시간 TR. 업종코드 001 코스피 종합, 301 코스닥 종합.
var lsSubscriptions = []ls.Subscription{{TrCd: "NWS", TrKey: "NWS001"}, {TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"}}

var lsMarkets = map[string]string{"001": "kospi", "301": "kosdaq"}

// lsToMsg 는 ls 이벤트를 화면 메시지로 바꾼다. 표시할 게 없으면 nil.
func lsToMsg(ev ls.Event) tea.Msg {
	switch e := ev.(type) {
	case ls.News:
		return NewsMsg{Title: e.Title}
	case ls.Index:
		market, ok := lsMarkets[e.Code]
		if !ok {
			return nil
		}
		return IndexMsg{Market: market, Value: e.Value, ChangePct: e.ChangePct}
	case ls.Disconnected:
		return DisconnectedMsg{}
	}
	return nil
}
```

- [ ] **Step 7: run.go 배선**

`internal/tui/run.go` 전체를 이렇게 바꾼다:

```go
package tui

import (
	"context"
	"errors"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/ls"
)

// Run 은 전체 화면 TUI 를 띄우고 종료될 때까지 막는다.
// LS 앱키가 있으면 실시간 뉴스·지수를 구독해 화면에 밀어 넣는다. 로그는 3단계에서 파일로 보낸다.
func Run(ctx context.Context, cfg *config.Config) error {
	// 취소 가능한 자식 컨텍스트: Run 안에서 시작하는 고루틴들이 q 로 TUI 종료 시 함께 멈추도록.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithContext(ctx))

	if cfg.LS.HasAppKey() {
		client := ls.New(ls.Config{
			BaseURL: cfg.LS.BaseURL, WSURL: cfg.LS.WSURL,
			AppKey: cfg.LS.AppKey, AppSecret: cfg.LS.AppSecret, TokenCache: cfg.LS.TokenCache,
		}, slog.New(slog.DiscardHandler))
		events := make(chan ls.Event, 64)
		go client.Run(ctx, lsSubscriptions, events)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ev := <-events:
					if msg := lsToMsg(ev); msg != nil {
						p.Send(msg)
					}
				}
			}
		}()
	}

	go func() {
		for _, msg := range fakeMessages(time.Now()) {
			p.Send(msg)
		}
	}()
	_, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil // Ctrl+C 로 컨텍스트가 취소된 정상 종료
	}
	return err
}
```

- [ ] **Step 8: fake.go 에서 뉴스·지수 삭제**

`internal/tui/fake.go` 에서 `NewsMsg{...}` 한 줄과 `IndexMsg{...}` 두 줄을 지우고, 주석을 이렇게 바꾼다:

```go
// fakeMessages 는 아직 실데이터가 없는 패널의 화면 확인용 가짜 데이터다.
// 3단계(로그), 7단계(관심종목), 8단계(보유종목)에서 실데이터로 바꾸며 해당 항목을 지우고, 다 지워지면 이 파일을 삭제한다.
```

- [ ] **Step 9: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... 2>&1 | tail -5`
Expected: gofmt 출력 없음. `config`, `ls`, `tui` 모두 `ok`. tui 는 기존 18 + 4 = 22개 (서브테스트 제외).

- [ ] **Step 10: 커밋 요청**

```bash
git add internal/tui
git commit -m "feat(tui): LS 실시간 뉴스·지수 연결, 미연결 처리"
```

---

### Task 4: ls-probe 서브커맨드와 실서버 수동 확인

**Files:**
- Modify: `cmd/trader/main.go`

**Interfaces:**
- Consumes: `config.Config.LS`, `ls.New`, `(*ls.Client).Run`, `ls.Subscription`.
- Produces: `trader ls-probe [초]` — LS 에 연결해 받은 이벤트를 stderr/stdout 에 찍고 N초(기본 30) 후 종료. TUI 가 오류를 가리므로 연결 문제를 볼 때 쓴다. 이후 단계에서도 유지. `JIF`(장운영정보)는 아직 파싱하지 않으므로 `parseMessage`가 모르는 TR 을 `ls.Raw{TrCd, TrKey, Body}` 로 넘기도록 Task 2 의 parse.go 에 한 케이스를 더한다 (이 Task 의 Step 1a).

- [ ] **Step 1a: 모르는 TR 을 Raw 이벤트로**

`internal/ls/client.go` 이벤트 타입 뒤에 추가:

```go
// Raw 는 아직 전용 파서가 없는 TR 의 수신 본문. ls-probe 가 실제 형식을 볼 때 쓴다.
type Raw struct {
	TrCd, TrKey string
	Body        map[string]any
}

func (Raw) isEvent() {}
```

`internal/ls/parse.go` 의 `parseMessage` 마지막 `return nil, false` 를 이렇게 바꾼다 (body 가 있는 모르는 TR 만 Raw, 구독 응답처럼 body 없는 것은 그대로 무시):

```go
	default:
		if m.Header.TrCd == "" {
			return nil, false
		}
		return Raw{TrCd: m.Header.TrCd, TrKey: m.Header.TrKey, Body: m.Body}, true
	}
```

`internal/ls/parse_test.go` 의 `TestParseIgnoresOthers` 에서 `S3_` 케이스를 빼고 아래 테스트를 추가한다:

```go
func TestParseUnknownTRAsRaw(t *testing.T) {
	ev, ok := parseMessage([]byte(`{"header":{"tr_cd":"JIF","tr_key":"1"},"body":{"jangubun":"1","jstatus":"21"}}`))
	if !ok {
		t.Fatal("expected Raw event")
	}
	r, isRaw := ev.(Raw)
	if !isRaw || r.TrCd != "JIF" || r.TrKey != "1" || r.Body["jstatus"] != "21" {
		t.Errorf("raw = %+v", ev)
	}
}
```

`internal/tui/ls.go` 의 `lsToMsg` 는 `default` 로 nil 을 돌려주므로 Raw 는 화면에 영향 없다.

Run: `go test ./internal/ls/ ./internal/tui/ 2>&1 | tail -3`
Expected: 전부 ok

- [ ] **Step 1: main.go 에 서브커맨드 추가**

`cmd/trader/main.go` 를 이렇게 바꾼다 (usage 와 switch 에 `ls-probe` 추가, `runLSProbe` 함수 추가):

```go
// trader 는 종가 베팅 운영 도구의 CLI 진입점이다.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/ls"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

const usage = `사용법:
  trader                 TUI
  trader ls-probe [초]   LS 실시간 이벤트를 N초(기본 30) 동안 출력 (연결 확인용)
  trader universe        (6단계 계획에서 구현)
  trader fetch           (6단계 계획에서 구현)
  trader watch           (7단계 계획에서 구현)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf(".env: %w", err)
	}
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return tui.Run(ctx, cfg)
	}
	switch args[0] {
	case "ls-probe":
		return runLSProbe(ctx, cfg, args[1:])
	case "universe", "fetch", "watch":
		return fmt.Errorf("%s 는 아직 구현되지 않았습니다", args[0])
	default:
		fmt.Print(usage)
		return fmt.Errorf("알 수 없는 명령 %q", args[0])
	}
}

// runLSProbe 는 LS 웹소켓에 연결해 받은 이벤트를 그대로 찍는다. 로그는 stderr, 이벤트는 stdout.
func runLSProbe(ctx context.Context, cfg *config.Config, args []string) error {
	if !cfg.LS.HasAppKey() {
		return fmt.Errorf("LS_APP_KEY / LS_APP_SECRET 환경변수가 없습니다 (.env 파일을 확인하세요)")
	}
	secs := 30
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("초는 양의 정수여야 합니다: %q", args[0])
		}
		secs = n
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(secs)*time.Second)
	defer cancel()

	client := ls.New(ls.Config{
		BaseURL: cfg.LS.BaseURL, WSURL: cfg.LS.WSURL,
		AppKey: cfg.LS.AppKey, AppSecret: cfg.LS.AppSecret, TokenCache: cfg.LS.TokenCache,
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// TUI 구독(tui.lsSubscriptions)에 더해 장운영정보 JIF 도 구독해 실제 코드 값을 본다 (3단계 이후 화면 표시 후보).
	subs := []ls.Subscription{
		{TrCd: "NWS", TrKey: "NWS001"},
		{TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"},
		{TrCd: "JIF", TrKey: "1"}, {TrCd: "JIF", TrKey: "2"},
	}
	events := make(chan ls.Event, 64)
	go client.Run(ctx, subs, events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			fmt.Printf("%s %T %+v\n", time.Now().Format("15:04:05"), ev, ev)
		}
	}
}
```

- [ ] **Step 2: 빌드와 전체 테스트**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... 2>&1 | tail -5`
Expected: 전부 통과

- [ ] **Step 3: 인자 검사 확인**

Run: `go run ./cmd/trader ls-probe abc; echo "exit=$?"`
Expected: `오류: 초는 양의 정수여야 합니다: "abc"` exit=1 (앱키 유무와 무관하게 앱키 검사가 먼저이므로, 앱키가 없으면 앱키 오류가 먼저 나온다 — 어느 쪽이든 exit=1)

- [ ] **Step 4: 실서버 수동 확인 (사용자, 장중 09:00~15:30 권장)**

Claude 는 `.env` 를 읽을 수 없으므로 실행 결과로만 확인한다. 사용자에게 아래를 실행하고 출력을 붙여 달라고 요청한다.

```bash
go run ./cmd/trader ls-probe 20
```

Expected:
1. stderr 에 `ls websocket 연결 subs=3`.
2. stdout 에 `ls.Connected {}` 한 줄, 이어서 장중이면 몇 초 안에 `ls.Index {Code:001 Value:2xxx.xx …}` 와 `{Code:301 Value:xxx.xx …}` 가 반복. `001` 값이 수천대, `301` 값이 수백대면 업종코드가 맞다. 반대거나 한쪽만 오면 `internal/tui/ls.go` 의 `lsSubscriptions`·`lsMarkets` 와 `runLSProbe` 의 `subs` 코드를 고친다.
3. 뉴스는 `ls.News {… Title:…}` 로 온다 (장중 빈도 높음, 장외엔 드묾).
3a. 장운영정보는 `ls.Raw {TrCd:JIF TrKey:1 Body:map[jangubun:1 jstatus:xx]}` 로 온다. 장 상태가 바뀔 때(09:00 시작, 15:20 동시호가, 15:30 마감 등)만 오므로 그 시각 근처에 돌려야 보인다. 받은 `jstatus` 값과 시각을 기록해 두면 3단계에서 화면 표시(장전/장중/마감)로 쓴다.
4. 토큰 발급 실패(HTTP 4xx)면 오류 메시지를 사용자에게 그대로 보여준다 — 앱키·시크릿 또는 포털의 API 신청 상태 문제.
5. 장외 시간이면 `Connected` 만 오고 지수는 안 온다. 그래도 연결·구독 형식은 검증된 것이다.

확인 후 TUI:

```bash
go run ./cmd/trader
```

Expected: 상단에 실제 뉴스 제목(출처 없이), 하단에 실제 지수. 뉴스가 아직 없으면 `미연결` 이 아니라 빈 상태여야 하는데 현재 구현은 뉴스 첫 수신 전까지 `미연결` 을 보인다 — 이는 알려진 동작이며 3단계에서 `Connected` 를 화면 상태로 반영할지 결정한다. 와이파이를 껐다 켜면 지수가 `미연결` 로 갔다가 돌아온다.

- [ ] **Step 5: 커밋 요청**

```bash
git add cmd/trader internal/ls
git commit -m "feat(cli): ls-probe 서브커맨드, JIF 장운영정보 구독"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- `trader ls-probe` 가 실서버에서 `Connected` 와 지수 이벤트를 받고, 업종코드 001/301 이 코스피/코스닥으로 확인됨
- `trader` 화면 상단·하단이 실데이터로 갱신됨
- 다음 단계(3단계 로그)는 `run.go` 의 `slog.New(slog.DiscardHandler)` 를 파일 로거로 바꾸고, `ls` 클라이언트의 연결 로그가 로그 패널에 보이게 한다
