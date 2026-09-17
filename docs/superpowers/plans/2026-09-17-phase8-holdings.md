# 8단계: 보유종목·잔고 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** TUI 보유종목 패널이 한투 잔고 조회로 실계좌(또는 `trade_env`에 따라 모의계좌)의 보유 종목·평가금액·예수금·손익을 보여주고, 장중엔 `balance_poll_seconds`마다 갱신된다. 마지막 가짜 데이터(`fake.go`)가 사라진다.

**Architecture:** `market.Status`를 타입으로 바꿔 폴링 시간대 판정(`Trading()`)에 쓴다. `kis`에 잔고 조회(`Balance`, 페이지 이어받기)와 계좌번호 파싱을 추가하고, `tui/holdings.go`가 매매용 클라이언트로 폴링해 `HoldingsMsg`로 보낸다. 시장 구분은 `symbols` 테이블에서 찾는다. TUI 는 표시만.

**Tech Stack:** 표준 라이브러리만. 새 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (5절 `trade_env`·`balance_poll_seconds`, 7.6절 잔고 조회, 10.1절 보유종목 패널, 11절)

**선행:** 7단계 완료. `.env`에 `KIS_REAL_ACCOUNT`(또는 `KIS_DEMO_ACCOUNT`), `config.yaml`의 `trade_env`.

## Global Constraints

- 모듈 경로 `github.com/gong-yeongbin/my-trading`. 새 의존성 없음. 테스트는 표준 `testing` + `httptest`.
- 잔고 API(스펙 7.6): `GET /uapi/domestic-stock/v1/trading/inquire-balance`, `tr_id` 실전 `TTTC8434R` / 모의 `VTTC8434R`. 파라미터 `CANO`(계좌 앞 8자리), `ACNT_PRDT_CD`(뒤 2자리), `AFHR_FLPR_YN=N`, `OFL_YN=`, `INQR_DVSN=02`, `UNPR_DVSN=01`, `FUND_STTL_ICLD_YN=N`, `FNCG_AMT_AUTO_RDPT_YN=N`, `PRCS_DVSN=01`, `CTX_AREA_FK100`, `CTX_AREA_NK100`. 응답 `output1[]`: `pdno`, `prdt_name`, `hldg_qty`, `pchs_avg_pric`, `prpr`, `evlu_pfls_amt`, `evlu_pfls_rt`(%); `output2[0]`: `tot_evlu_amt`, `dnca_tot_amt`(예수금), `evlu_pfls_smtl_amt`; `ctx_area_fk100`, `ctx_area_nk100`. 응답 헤더 `tr_cont`가 `F` 또는 `M`이면 다음 페이지가 있고, 요청 헤더 `tr_cont: N` + 위 두 CTX 값으로 이어받는다. 최대 20페이지.
- 숫자 필드는 문자열(`"71200.00"`, `"2.40"`)로 온다. `float64`로 파싱해 원 단위 정수는 반올림, 수익률은 `/100`. 수량 0 행은 건너뛴다.
- 계좌번호 `12345678-01` 형식만 허용. 매매용 클라이언트는 `trade_env`에 따라 `TradeBaseURL()`·`Trade` 키·`TradeTokenCache`·`TradeRPS()`.
- 폴링: 시작 시 1회. 이후 `balance_poll_seconds`마다 `market.At(now).Trading()`(장중·동시호가)이면 조회, 거래 시간대가 끝난 직후 1회 더(종가 반영). 실패하면 `kind=매매` Warn 로그, 마지막 성공 값 유지; 한 번도 성공 못 했으면 `미연결`.
- 보유종목 정렬: 평가금액(현재가×수량) 큰 순. 보유일은 `-`(매매 로그는 나중 단계). 시장 구분은 `symbols` 테이블(코스피/코스닥), 없으면 `-`.
- `market.Status`는 타입(`int`) + `String()` + `Trading()`. 화면 문구는 변하지 않는다.
- TUI 는 로직 없음. 색 금지. 폭은 `lipgloss.Width`/`ansi.Truncate`.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/*.json` 읽기 금지. 실서버 호출 금지(테스트). `go run ./cmd/trader`(인자 없음) 실행 금지. `gofmt -l .` 비어야 함.

---

## 파일 구조

```
internal/market/market.go        Status 타입, At, FromJIF (수정)
internal/market/market_test.go   (수정)
internal/tui/types.go            MarketStatusMsg.Status 타입 (수정)
internal/tui/model.go            jifStatus 타입 (수정)
internal/tui/view.go             .String() (수정)
internal/tui/run.go              FromJIF 타입, 잔고 폴링 배선 (수정)
internal/kis/client.go           do 가 tr_cont 헤더 반환, getCont (수정)
internal/kis/balance.go          Account, ParseAccount, Position, Balance, (*Client).Balance
internal/kis/balance_test.go
internal/tui/holdings.go         holdingsMsgFrom, holdingsLoop
internal/tui/holdings_test.go
internal/tui/fake.go             삭제
internal/tui/model_test.go, view_test.go (수정)
```

---

### Task 1: `market.Status` 타입화

**Files:**
- Modify: `internal/market/market.go`, `internal/market/market_test.go`, `internal/tui/types.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/run.go`, `internal/tui/model_test.go`, `internal/tui/view_test.go`

**Interfaces:**
- Produces: `market.Status` (`Closed, PreOpen, Open, ClosingAuction, AfterClose, AfterHours`), `(Status).String()`, `(Status).Trading() bool`, `market.At(t time.Time) Status` (구 `Status(t)`), `market.FromJIF(code) (Status, bool)`. `tui.MarketStatusMsg{Status market.Status}`. Task 3 이 `At(now).Trading()` 을 쓴다.

- [ ] **Step 1: market 테스트 갱신**

`internal/market/market_test.go`: `Status(tc.t)` 호출을 `At(tc.t).String()` 으로, `FromJIF` 기대를 `.String()` 비교로 바꾼다. 추가:

```go
func TestTrading(t *testing.T) {
	for s, want := range map[Status]bool{Closed: false, PreOpen: false, Open: true, ClosingAuction: true, AfterClose: false, AfterHours: false} {
		if s.Trading() != want {
			t.Errorf("%s.Trading() = %v", s, s.Trading())
		}
	}
	if Status(99).String() != "알 수 없음" {
		t.Error("unknown status string")
	}
}
```

- [ ] **Step 2: market 구현**

`internal/market/market.go`:

```go
// Status 는 장 상태. 문자열이 아니라 타입으로 두어 폴링 시간대 판정 등에서 문구 비교를 하지 않게 한다.
type Status int

const (
	Closed         Status = iota // 휴장
	PreOpen                      // 장전
	Open                         // 장중
	ClosingAuction               // 동시호가
	AfterClose                   // 장마감
	AfterHours                   // 시간외
)

var statusNames = [...]string{"휴장", "장전", "장중", "동시호가", "장마감", "시간외"}

func (s Status) String() string {
	if s < 0 || int(s) >= len(statusNames) {
		return "알 수 없음"
	}
	return statusNames[s]
}

// Trading 은 정규장 거래 시간대(장중·동시호가)인지. 잔고 폴링 등에 쓴다.
func (s Status) Trading() bool { return s == Open || s == ClosingAuction }

// At 은 t 시각의 장 상태 (KST, 공휴일 미반영).
func At(t time.Time) Status {
	t = t.In(KST)
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return Closed
	}
	hm := t.Hour()*60 + t.Minute()
	switch {
	case hm < 9*60:
		return PreOpen
	case hm < 15*60+20:
		return Open
	case hm < 15*60+30:
		return ClosingAuction
	case hm < 16*60:
		return AfterClose
	case hm < 18*60:
		return AfterHours
	default:
		return AfterClose
	}
}

var jifStatus = map[string]Status{"11": PreOpen, "21": Open, "31": ClosingAuction, "41": AfterClose, "51": AfterHours, "52": AfterClose, "61": AfterHours, "62": AfterClose}

func FromJIF(code string) (Status, bool) { s, ok := jifStatus[code]; return s, ok }
```

- [ ] **Step 3: tui 갱신**

- `types.go`: `MarketStatusMsg{Status market.Status}` (import `market`).
- `model.go`: `jifStatus market.Status` + `jifKnown bool` (`jifStatus != ""` 검사를 `jifKnown` 으로). `case MarketStatusMsg: m.jifStatus, m.jifKnown, m.jifAt = msg.Status, true, m.now`. `marketStatus() market.Status` — 같은 날이면 `m.jifStatus`, 아니면 `market.At(m.now)`.
- `view.go` `footerLeft`: `" [" + m.marketStatus().String() + "] …"`.
- `run.go`: `status, known := market.FromJIF(ms.Code)`; 로그 `"status", status.String()`; `p.Send(MarketStatusMsg{Status: status})`.
- 테스트: `model_test.go` 의 `MarketStatusMsg{Status: "동시호가"}` → `market.ClosingAuction`. 나머지 문구 검사는 그대로 통과해야 한다.

- [ ] **Step 4: 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... -count=1 2>&1 | tail -6`
Expected: 전부 ok.

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/market internal/tui
git commit -m "refactor(market): 장 상태를 타입으로, Trading() 판정"
```

---

### Task 2: 한투 잔고 조회 (internal/kis)

**Files:**
- Modify: `internal/kis/client.go`
- Create: `internal/kis/balance.go`, `internal/kis/balance_test.go`

**Interfaces:**
- Produces: `kis.Account{CANO, Product string}`, `kis.ParseAccount(s string) (Account, error)`, `kis.Position{Code, Name string; Qty, AvgPrice, Price, PnL int64; PnLPct float64}`, `kis.Balance{Positions []Position; Total, Cash, PnL int64}`, `(*Client).Balance(ctx, acct Account, demo bool) (Balance, error)`. 비공개 `getCont(ctx, path, trID, trCont string, params, out) (nextCont string, err error)`; `get` 은 `getCont(…, "", …)` 로 구현. `do` 가 `(status int, trCont string, body []byte, err)` 를 돌려주고 요청 헤더 `tr_cont` 를 받는다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/kis/balance_test.go`:

```go
package kis

import (
	"context"
	"net/http"
	"testing"
)

func TestParseAccount(t *testing.T) {
	a, err := ParseAccount("12345678-01")
	if err != nil || a.CANO != "12345678" || a.Product != "01" {
		t.Errorf("parse = %+v %v", a, err)
	}
	for _, bad := range []string{"", "1234567801", "12345678-1", "1234567-01", "abcdefgh-01"} {
		if _, err := ParseAccount(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

const balancePage1 = `{"rt_cd":"0","msg_cd":"KIOK0510","msg1":"조회되었습니다",
 "ctx_area_fk100":"FK1","ctx_area_nk100":"NK1",
 "output1":[
  {"pdno":"005930","prdt_name":"삼성전자","hldg_qty":"58","pchs_avg_pric":"71200.0000","prpr":"72900","evlu_pfls_amt":"98600","evlu_pfls_rt":"2.40"},
  {"pdno":"000660","prdt_name":"SK하이닉스","hldg_qty":"0","pchs_avg_pric":"0.0000","prpr":"180000","evlu_pfls_amt":"0","evlu_pfls_rt":"0.00"}
 ],
 "output2":[{"tot_evlu_amt":"12480000","dnca_tot_amt":"7520000","evlu_pfls_smtl_amt":"312000"}]}`

const balancePage2 = `{"rt_cd":"0","msg_cd":"KIOK0510","msg1":"조회되었습니다",
 "ctx_area_fk100":"","ctx_area_nk100":"",
 "output1":[{"pdno":"247540","prdt_name":"에코프로비엠","hldg_qty":"40","pchs_avg_pric":"98500.5000","prpr":"96100","evlu_pfls_amt":"-96020","evlu_pfls_rt":"-2.44"}],
 "output2":[{"tot_evlu_amt":"12480000","dnca_tot_amt":"7520000","evlu_pfls_smtl_amt":"312000"}]}`

func TestBalancePagesAndParses(t *testing.T) {
	f := newFakeKIS(t)
	var calls []*http.Request
	f.handle("/uapi/domestic-stock/v1/trading/inquire-balance", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r)
		switch r.Header.Get("tr_cont") {
		case "":
			w.Header().Set("tr_cont", "M")
			w.Write([]byte(balancePage1))
		case "N":
			w.Header().Set("tr_cont", "D")
			w.Write([]byte(balancePage2))
		default:
			t.Errorf("unexpected tr_cont %q", r.Header.Get("tr_cont"))
		}
	})
	c := f.client
	b, err := c.Balance(context.Background(), Account{CANO: "12345678", Product: "01"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	q := calls[0].URL.Query()
	for k, want := range map[string]string{"CANO": "12345678", "ACNT_PRDT_CD": "01", "AFHR_FLPR_YN": "N", "INQR_DVSN": "02", "UNPR_DVSN": "01", "FUND_STTL_ICLD_YN": "N", "FNCG_AMT_AUTO_RDPT_YN": "N", "PRCS_DVSN": "01", "CTX_AREA_FK100": "", "CTX_AREA_NK100": ""} {
		if got := q.Get(k); got != want {
			t.Errorf("param %s = %q, want %q", k, got, want)
		}
	}
	if calls[0].Header.Get("tr_id") != "TTTC8434R" {
		t.Errorf("tr_id = %q", calls[0].Header.Get("tr_id"))
	}
	q2 := calls[1].URL.Query()
	if q2.Get("CTX_AREA_FK100") != "FK1" || q2.Get("CTX_AREA_NK100") != "NK1" {
		t.Errorf("continuation params = %v", q2)
	}
	if len(b.Positions) != 2 { // 수량 0 제외
		t.Fatalf("positions = %+v", b.Positions)
	}
	p := b.Positions[0]
	if p.Code != "005930" || p.Name != "삼성전자" || p.Qty != 58 || p.AvgPrice != 71200 || p.Price != 72900 || p.PnL != 98600 || p.PnLPct != 0.024 {
		t.Errorf("position = %+v", p)
	}
	if b.Positions[1].AvgPrice != 98501 || b.Positions[1].PnL != -96020 { // 98500.5 반올림
		t.Errorf("position 2 = %+v", b.Positions[1])
	}
	if b.Total != 12480000 || b.Cash != 7520000 || b.PnL != 312000 {
		t.Errorf("summary = %+v", b)
	}
}

func TestBalanceDemoTrID(t *testing.T) {
	f := newFakeKIS(t)
	var trID string
	f.handle("/uapi/domestic-stock/v1/trading/inquire-balance", func(w http.ResponseWriter, r *http.Request) {
		trID = r.Header.Get("tr_id")
		w.Write([]byte(balancePage2))
	})
	if _, err := f.client.Balance(context.Background(), Account{CANO: "12345678", Product: "01"}, true); err != nil {
		t.Fatal(err)
	}
	if trID != "VTTC8434R" {
		t.Errorf("demo tr_id = %q", trID)
	}
}

func TestBalanceBusinessError(t *testing.T) {
	f := newFakeKIS(t)
	f.handle("/uapi/domestic-stock/v1/trading/inquire-balance", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기타오류"}`))
	})
	if _, err := f.client.Balance(context.Background(), Account{CANO: "12345678", Product: "01"}, false); err == nil {
		t.Error("expected error")
	}
}
```
`newFakeKIS(t)`, `f.handle(path, h)`, 필드 `f.client`(*Client) 는 `client_test.go` 의 기존 헬퍼다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/kis/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: ParseAccount`)

- [ ] **Step 3: client.go 수정**

- `do` 시그니처를 `do(ctx, path, trID, trCont string, params url.Values, tok string) (status int, respCont string, body []byte, err error)` 로. `trCont != ""` 이면 요청 헤더 `tr_cont` 설정. 반환에 `resp.Header.Get("tr_cont")` 추가.
- `get` 을 `getCont(ctx, path, trID, trCont string, params, out any) (string, error)` 로 바꾸고 (성공 시 응답 `tr_cont` 반환), `get` 은 `_, err := c.getCont(ctx, path, trID, "", params, out); return err`. 기존 호출처(`chart.go`, 테스트)는 `get` 그대로.

- [ ] **Step 4: balance.go**

```go
package kis

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
)

const balancePath = "/uapi/domestic-stock/v1/trading/inquire-balance"

// Account 는 한투 계좌번호 "12345678-01" 을 나눈 것.
type Account struct {
	CANO    string // 앞 8자리
	Product string // 뒤 2자리 (계좌상품코드)
}

var accountRe = regexp.MustCompile(`^(\d{8})-(\d{2})$`)

func ParseAccount(s string) (Account, error) {
	m := accountRe.FindStringSubmatch(s)
	if m == nil {
		return Account{}, fmt.Errorf("kis: 계좌번호는 12345678-01 형식이어야 합니다: %q", s)
	}
	return Account{CANO: m[1], Product: m[2]}, nil
}

// Position 은 보유 종목 한 줄. 금액은 원 단위 정수(반올림), PnLPct 는 0.024 = +2.4%.
type Position struct {
	Code, Name string
	Qty        int64
	AvgPrice   int64
	Price      int64
	PnL        int64
	PnLPct     float64
}

// Balance 는 계좌 잔고. Total 평가금액, Cash 예수금, PnL 평가손익 합계.
type Balance struct {
	Positions []Position
	Total     int64
	Cash      int64
	PnL       int64
}

type balanceResp struct {
	CtxFK   string `json:"ctx_area_fk100"`
	CtxNK   string `json:"ctx_area_nk100"`
	Output1 []struct {
		Code     string `json:"pdno"`
		Name     string `json:"prdt_name"`
		Qty      string `json:"hldg_qty"`
		AvgPrice string `json:"pchs_avg_pric"`
		Price    string `json:"prpr"`
		PnL      string `json:"evlu_pfls_amt"`
		PnLPct   string `json:"evlu_pfls_rt"`
	} `json:"output1"`
	Output2 []struct {
		Total string `json:"tot_evlu_amt"`
		Cash  string `json:"dnca_tot_amt"`
		PnL   string `json:"evlu_pfls_smtl_amt"`
	} `json:"output2"`
}

// Balance 는 잔고를 조회한다. demo 면 모의투자 tr_id. 다음 페이지가 있으면 이어받는다 (최대 20페이지).
func (c *Client) Balance(ctx context.Context, acct Account, demo bool) (Balance, error) {
	trID := "TTTC8434R"
	if demo {
		trID = "VTTC8434R"
	}
	var out Balance
	fk, nk, cont := "", "", ""
	for page := 0; page < 20; page++ {
		params := url.Values{
			"CANO": {acct.CANO}, "ACNT_PRDT_CD": {acct.Product},
			"AFHR_FLPR_YN": {"N"}, "OFL_YN": {""}, "INQR_DVSN": {"02"}, "UNPR_DVSN": {"01"},
			"FUND_STTL_ICLD_YN": {"N"}, "FNCG_AMT_AUTO_RDPT_YN": {"N"}, "PRCS_DVSN": {"01"},
			"CTX_AREA_FK100": {fk}, "CTX_AREA_NK100": {nk},
		}
		var resp balanceResp
		next, err := c.getCont(ctx, balancePath, trID, cont, params, &resp)
		if err != nil {
			return Balance{}, err
		}
		for _, r := range resp.Output1 {
			qty := num(r.Qty)
			if qty <= 0 {
				continue
			}
			out.Positions = append(out.Positions, Position{
				Code: r.Code, Name: r.Name, Qty: qty,
				AvgPrice: num(r.AvgPrice), Price: num(r.Price), PnL: num(r.PnL),
				PnLPct: fnum(r.PnLPct) / 100,
			})
		}
		if len(resp.Output2) > 0 {
			out.Total, out.Cash, out.PnL = num(resp.Output2[0].Total), num(resp.Output2[0].Cash), num(resp.Output2[0].PnL)
		}
		if next != "F" && next != "M" {
			break
		}
		fk, nk, cont = resp.CtxFK, resp.CtxNK, "N"
	}
	return out, nil
}

func fnum(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func num(s string) int64 { return int64(math.Round(fnum(s))) }
```
(`strings` import 추가.)

- [ ] **Step 5: 확인**

Run: `gofmt -l . ; go vet ./internal/kis/ && go test ./internal/kis/ -count=1 -race 2>&1 | tail -3`
Expected: 기존 + 새 테스트 4개 ok.

- [ ] **Step 6: 커밋 요청**

```bash
git add internal/kis
git commit -m "feat(kis): 잔고 조회 (페이지 이어받기), 계좌번호 파싱"
```

---

### Task 3: TUI 보유종목 폴링

**Files:**
- Create: `internal/tui/holdings.go`, `internal/tui/holdings_test.go`
- Modify: `internal/tui/run.go`, `internal/tui/fake.go`(삭제), `internal/tui/model_test.go`(필요 시)

**Interfaces:**
- Consumes: `kis.Balance`, `kis.ParseAccount`, `config.KIS.Trade*`, `RequireTradeKey`, `market.At(...).Trading()`, `data.Store.ListSymbols`.
- Produces: 비공개 `holdingsMsgFrom(b kis.Balance, market func(code string) string) HoldingsMsg`, `holdingsLoop(ctx, every time.Duration, now func() time.Time, fetch func(context.Context) (HoldingsMsg, error), send func(tea.Msg), logger *slog.Logger)`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/holdings_test.go`:

```go
package tui

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/market"
)

func TestHoldingsMsgFromSortsAndMaps(t *testing.T) {
	b := kis.Balance{
		Positions: []kis.Position{
			{Code: "247540", Name: "에코프로비엠", Qty: 40, AvgPrice: 98500, Price: 96100, PnL: -96000, PnLPct: -0.024},
			{Code: "005930", Name: "삼성전자", Qty: 58, AvgPrice: 71200, Price: 72900, PnL: 98600, PnLPct: 0.024},
			{Code: "005380", Name: "현대차", Qty: 16, AvgPrice: 245000, Price: 264300, PnL: 308800, PnLPct: 0.079},
		},
		Total: 12480000, Cash: 7520000, PnL: 312000,
	}
	mk := map[string]string{"005930": "kospi", "247540": "kosdaq"}
	msg := holdingsMsgFrom(b, func(code string) string { return mk[code] })
	if !msg.Connected || msg.Summary.Total != 12480000 || msg.Summary.Cash != 7520000 || msg.Summary.PnL != 312000 {
		t.Errorf("summary = %+v", msg.Summary)
	}
	if msg.Summary.PnLPct < 0.0256 || msg.Summary.PnLPct > 0.0257 { // 312000 / (12480000-312000)
		t.Errorf("PnLPct = %v", msg.Summary.PnLPct)
	}
	// 평가금액 순: 현대차 4,228,800 > 삼성전자 4,228,200 > 에코프로비엠 3,844,000
	if len(msg.Rows) != 3 || msg.Rows[0].Name != "현대차" || msg.Rows[1].Name != "삼성전자" || msg.Rows[2].Name != "에코프로비엠" {
		t.Errorf("order = %+v", msg.Rows)
	}
	if msg.Rows[1].Market != "코스피" || msg.Rows[2].Market != "코스닥" || msg.Rows[0].Market != "-" {
		t.Errorf("markets = %+v", msg.Rows)
	}
	if msg.Rows[1].HoldDays != 0 || msg.Rows[1].Qty != 58 || msg.Rows[1].PnLPct != 0.024 {
		t.Errorf("row = %+v", msg.Rows[1])
	}
}

func TestHoldingsLoopPollsOnlyWhileTrading(t *testing.T) {
	var fetches atomic.Int32
	var msgs []tea.Msg
	send := func(m tea.Msg) { msgs = append(msgs, m) }
	// 시계: 처음 3틱은 장중(10:00), 다음 2틱은 장마감(15:35), 그 뒤 장마감 유지
	ticks := []time.Time{
		time.Date(2026, 9, 17, 10, 0, 0, 0, market.KST), time.Date(2026, 9, 17, 10, 0, 10, 0, market.KST), time.Date(2026, 9, 17, 10, 0, 20, 0, market.KST),
		time.Date(2026, 9, 17, 15, 35, 0, 0, market.KST), time.Date(2026, 9, 17, 15, 35, 10, 0, market.KST), time.Date(2026, 9, 17, 15, 35, 20, 0, market.KST),
	}
	i := 0
	now := func() time.Time {
		if i < len(ticks) {
			t := ticks[i]
			i++
			return t
		}
		return ticks[len(ticks)-1]
	}
	fetch := func(context.Context) (HoldingsMsg, error) { fetches.Add(1); return HoldingsMsg{Connected: true}, nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { holdingsLoop(ctx, 5*time.Millisecond, now, fetch, send, slog.New(slog.DiscardHandler)); close(done) }()
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done
	// 시작 1회(틱 0) + 장중 틱 1,2 = 2회 + 장중 종료 직후 틱 3 = 1회 → 4회. 틱 4·5(장마감 지속)는 없음.
	if got := fetches.Load(); got != 4 {
		t.Errorf("fetches = %d, want 4", got)
	}
	if len(msgs) != 4 {
		t.Errorf("msgs = %d", len(msgs))
	}
}

func TestHoldingsLoopKeepsLastOnErrorAndReportsFirstFailure(t *testing.T) {
	var msgs []tea.Msg
	send := func(m tea.Msg) { msgs = append(msgs, m) }
	now := func() time.Time { return time.Date(2026, 9, 17, 10, 0, 0, 0, market.KST) }
	calls := 0
	fetch := func(context.Context) (HoldingsMsg, error) {
		calls++
		if calls == 1 {
			return HoldingsMsg{}, errors.New("boom")
		}
		return HoldingsMsg{Connected: true, Rows: []HoldingRow{{Name: "삼성전자"}}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { holdingsLoop(ctx, 5*time.Millisecond, now, fetch, send, slog.New(slog.DiscardHandler)); close(done) }()
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done
	if len(msgs) < 2 {
		t.Fatalf("msgs = %d", len(msgs))
	}
	if first := msgs[0].(HoldingsMsg); first.Connected {
		t.Error("first failure with no prior data should send 미연결")
	}
	if second := msgs[1].(HoldingsMsg); !second.Connected || len(second.Rows) != 1 {
		t.Errorf("second = %+v", second)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: holdingsMsgFrom`)

- [ ] **Step 3: holdings.go**

```go
package tui

import (
	"context"
	"log/slog"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/market"
)

// holdingsMsgFrom 은 잔고를 화면 메시지로 바꾼다. 평가금액(현재가×수량) 큰 순. 보유일은 매매 로그 전까지 "-"(0).
func holdingsMsgFrom(b kis.Balance, mkt func(code string) string) HoldingsMsg {
	msg := HoldingsMsg{Connected: true, Summary: HoldingsSummary{Total: b.Total, PnL: b.PnL, Cash: b.Cash}}
	if base := b.Total - b.PnL; base > 0 {
		msg.Summary.PnLPct = float64(b.PnL) / float64(base)
	}
	for _, p := range b.Positions {
		name := marketNames[mkt(p.Code)]
		if name == "" {
			name = "-"
		}
		msg.Rows = append(msg.Rows, HoldingRow{Name: p.Name, Market: name, Qty: p.Qty, AvgPrice: p.AvgPrice, Price: p.Price, PnL: p.PnL, PnLPct: p.PnLPct})
	}
	sort.SliceStable(msg.Rows, func(i, j int) bool { return msg.Rows[i].Price*msg.Rows[i].Qty > msg.Rows[j].Price*msg.Rows[j].Qty })
	return msg
}

// holdingsLoop 은 시작 시 한 번, 이후 every 마다 거래 시간대(market.At(now).Trading())면 잔고를 조회한다.
// 거래 시간대가 끝난 직후 한 번 더 조회해 종가를 반영한다. 실패하면 로그만 남기고 마지막 값을 유지한다.
func holdingsLoop(ctx context.Context, every time.Duration, now func() time.Time, fetch func(context.Context) (HoldingsMsg, error), send func(tea.Msg), logger *slog.Logger) {
	ok := false
	poll := func() {
		msg, err := fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("잔고 조회 실패", "err", err)
			if !ok {
				send(HoldingsMsg{Connected: false})
			}
			return
		}
		ok = true
		send(msg)
	}
	poll()
	wasTrading := market.At(now()).Trading()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		trading := market.At(now()).Trading()
		if trading || wasTrading {
			poll()
		}
		wasTrading = trading
	}
}
```
(`marketNames` 는 `screen.go` 에 있다.) 시작 시 `poll()` 뒤 `now()` 를 한 번 더 부르므로 테스트의 시계 시퀀스: poll 은 now 를 안 부르고, 그 뒤 `wasTrading` 계산이 틱 0 을 소비한다 — 테스트 기대(4회)는 이 순서를 전제한다.

- [ ] **Step 4: run.go 배선, fake.go 삭제**

`run.go` 의 자동 수집 블록 뒤(같은 `else` 안, store 가 열린 뒤)에:

```go
		// 보유종목: 매매 서버(trade_env)의 계좌를 폴링한다.
		holdLog := logger.With("kind", "매매")
		if err := cfg.RequireTradeKey(); err != nil {
			holdLog.Warn("보유종목 비활성: " + err.Error())
			go p.Send(HoldingsMsg{Connected: false})
		} else if acct, err := kis.ParseAccount(cfg.KIS.Trade.Account); err != nil {
			holdLog.Warn("보유종목 비활성: " + err.Error())
			go p.Send(HoldingsMsg{Connected: false})
		} else {
			trade := kis.New(cfg.KIS.TradeBaseURL(), cfg.KIS.Trade.AppKey, cfg.KIS.Trade.AppSecret, cfg.KIS.TradeTokenCache, cfg.KIS.TradeRPS())
			demo := cfg.KIS.TradeEnv != "real"
			fetch := func(c context.Context) (HoldingsMsg, error) {
				b, err := trade.Balance(c, acct, demo)
				if err != nil {
					return HoldingsMsg{}, err
				}
				syms, _ := store.ListSymbols(c)
				mk := map[string]string{}
				for _, s := range syms {
					mk[s.Code] = s.Market
				}
				return holdingsMsgFrom(b, func(code string) string { return mk[code] }), nil
			}
			go holdingsLoop(ctx, time.Duration(cfg.KIS.BalancePollSeconds)*time.Second, time.Now, fetch, p.Send, holdLog)
		}
```
`RequireTradeKey` 오류 문구가 `KIS_DEMO_*`/`KIS_REAL_*` 를 말하는지 확인 (Task 6.7 때 만든 것). `cfg.KIS.Trade.Account` 가 비어 있으면 `ParseAccount` 가 거부한다 — 문구에 "설정 메뉴에서 계좌번호 입력" 을 덧붙인다.

`fake.go` 와 `run.go` 의 `fakeMessages` 고루틴을 삭제한다. `time` import 가 남는지 확인.

- [ ] **Step 5: 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... -count=1 -race 2>&1 | tail -8`
Expected: 전부 ok. `internal/tui/fake.go` 없음.

- [ ] **Step 6: 커밋 요청**

```bash
git add internal/tui
git commit -m "feat(tui): 보유종목 실계좌 잔고 폴링, 가짜 데이터 제거"
```

---

### Task 4: 스펙 갱신과 화면 확인

- [ ] **Step 1: 스펙**

7.6절: 페이지 이어받기(`tr_cont`) 문장 추가; "보유일 `-`" 유지. 10.1절 보유종목 행: 갱신 규칙을 "시작 시 1회, 장중(장중·동시호가) `balance_poll_seconds`마다, 거래 종료 직후 1회" 로; 정렬 "평가금액 큰 순". 11절: "매매 키·계좌번호 없으면 보유종목 `미연결` + `kind=매매` 로그". 3절 파일 구조에서 `fake.go` 언급 있으면 삭제.

- [ ] **Step 2: 화면 확인 (사용자)**

1. 설정 메뉴에서 `매매 서버`를 `real`로, `한투 실전 계좌`에 `12345678-01` 형식 입력 → 재시작.
2. 보유종목 패널: 실제 보유 종목이 평가금액 순으로, 제목 줄에 평가금액·손익·현금. 보유가 없으면 `보유 없음`.
3. 로그 패널에 `[매매] …` 줄이 없어야 정상(실패 시에만 Warn).
4. 장중이면 현재가·손익이 10초마다 바뀐다. 장외면 시작 시 값 그대로.
5. `trade_env: demo` 로 바꾸면 모의계좌(`KIS_DEMO_ACCOUNT`) 잔고.

- [ ] **Step 3: 커밋 요청**

```bash
git add docs
git commit -m "docs: 8단계 반영"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `gofmt -l .` 비어 있음, `internal/tui/fake.go` 삭제됨
- 보유종목 패널이 실계좌 잔고를 보여주고 장중 갱신됨
- 이로써 설계 1절 범위(수집·선별·TUI)가 끝난다. 다음은 13절 나중 단계(주문, 매매 로그 → 보유일 등)
