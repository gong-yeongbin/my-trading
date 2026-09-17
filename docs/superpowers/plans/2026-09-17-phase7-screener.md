# 7단계: 전략 조건·관심종목 선별 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 수집된 일봉으로 "오늘 종가가 얼마 이상이면 진입 조건을 통과하는가"를 종목마다 계산해 `trader watch`로 표를 찍고, TUI 관심종목 패널이 실데이터로 바뀐다. 자동 수집이 끝나면 다시 계산한다. 하단 지수는 미연결일 때 전일 종가 `(전일)`을 보인다.

**Architecture:** `strategy`(설계 8절 진입 조건 순수 함수, `config.StrategyConfig`·`data` 만 의존) → `screener`(설계 9절 문턱값 대수식으로 `WatchItem` 계산; `Run(ctx, cfg, store)` 가 종목마다 봉을 읽어 목록·시장 필터 상태를 만듦) → `cmd/trader watch`(표 출력), `tui`(시작 시·수집 완료 후 `screener.Run` → `WatchMsg`). TUI 는 여전히 계산하지 않는다.

**Tech Stack:** 표준 라이브러리만. 새 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (8절, 9절, 10.1절 관심종목·하단, 12절 strategy·screener)

**선행:** 6.5단계까지 완료. `data/market.db` 에 1년치 일봉이 있으면 화면 확인 가능.

## Global Constraints

- 모듈 경로 `github.com/gong-yeongbin/my-trading`. 새 의존성 없음. 테스트는 표준 `testing`.
- 가격 `int64` 원, 거래대금 = `Close × Volume` (`int64`, 1e15 이하라 안전), 이동평균·비율은 `float64`.
- `strategy` 는 `config.StrategyConfig` 와 `data` 만 import. 상태 없음. 미래 봉을 받지 않는다 — 호출자가 잘라서 넘긴다.
- 진입 조건 8개는 설계 8.2절 그대로 (경계 포함/미포함 정확히): 1 지수 종가 > n일 평균(당일 포함, 봉 n개 이상); 2 봉 ≥ `ma_long_days + 1`; 3 거래대금 ≥ `min_turnover`; 4 거래대금 ≥ 직전 `turnover_ma_days`일 평균(당일 제외) × `turnover_ratio_min`; 5 종가 ≥ 고가 × `close_to_high_min`; 6 `change_min ≤ 등락률 < change_max`; 7 종가 > 단기MA > 장기MA(당일 포함); 8 종가 ≥ 직전 `new_high_days`일 고가 최댓값(당일 제외).
- 문턱값(설계 9.1): `P` 전일 종가, `S_{n-1}` 직전 n−1일 종가 합, `S_{m-1}` 직전 m−1일 종가 합, `H` 직전 `new_high_days`일 고가 최댓값, `T` 직전 `turnover_ma_days`일 평균 거래대금. 하한: 상승률 `ceil(P·(1+change_min) − 1e-6)`(부동소수 보정); 신고가 `H`; 단기선 `floor(S_{n-1}/(n-1))+1`; 장기선 `floor((n·S_{m-1} − m·S_{n-1})/(m−n))+1` (0 미만이면 0); `C_min` = 최댓값. `T_req = max(min_turnover, ratio·T)`, `MinVolume = ceil(T_req / C_min)`. 실현 가능: `C_min < P·(1+change_max)`. 봉이 `m` 개 미만이면 대상 아님.
- 정렬: `MinChangePct` 오름차순, 같으면 코드 오름차순.
- "전일" = 저장소의 코스피 지수 마지막 봉 날짜(`LastIndexBarDate`). 종목 봉은 그 날짜까지만 쓴다(그 뒤 날짜가 있어도 자름).
- 시장 필터 상태: 지수 봉 n개 이상이고 마지막 종가 > n일 평균이면 `진입가능`, 아니면 `차단`, 봉 부족이면 `알 수 없음`.
- TUI 는 로직 없음. `screener.Run` 결과를 `WatchMsg` 로 바꿀 뿐. 색 금지. 폭은 `lipgloss.Width`/`ansi.Truncate`.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/*.json` 읽기 금지. 테스트는 임시 SQLite. `go run ./cmd/trader`(인자 없음) 실행 금지. `gofmt -l .` 비어야 함.

---

## 파일 구조

```
internal/strategy/strategy.go      Market, Entry, 조건 함수 6개, Score, 이동평균 헬퍼
internal/strategy/strategy_test.go 경계값 테스트
internal/screener/screener.go      WatchItem, Compute, Result, Run, FilterStatus
internal/screener/screener_test.go 대수식 검증(무작위), 정렬, 실현 불가 제외, Run(임시 DB)
cmd/trader/main.go                 watch 서브커맨드 (수정)
internal/tui/types.go              IndexPrevCloseMsg (수정)
internal/tui/model.go              prevClose 저장 (수정)
internal/tui/view.go               fmtIndex (전일) (수정)
internal/tui/screen.go             screener.Result → WatchMsg, runScreen
internal/tui/screen_test.go
internal/tui/autofetch.go          runFetchOnce 에 onDone 훅 (수정)
internal/tui/run.go                시작 시·수집 후 runScreen, 전일 지수 전송 (수정)
internal/tui/fake.go               관심종목 항목 삭제 (수정)
internal/tui/model_test.go, view_test.go (수정)
```

---

### Task 1: 전략 조건 함수 (internal/strategy)

**Files:**
- Create: `internal/strategy/strategy.go`, `internal/strategy/strategy_test.go`

**Interfaces:**
- Produces: `strategy.Market{IndexBars []data.IndexBar}`, `strategy.Entry(cfg config.StrategyConfig, mkt Market, bars []data.Bar) bool`, `MarketFilter(cfg, mkt) bool`, `Liquidity(cfg, bars) bool`, `CloseStrength(cfg, bars) bool`, `Change(cfg, bars) bool`, `Trend(cfg, bars) bool`, `NewHigh(cfg, bars) bool`, `Score(cfg, bars) float64`, `Turnover(b data.Bar) int64`. Task 2 의 검증 테스트가 개별 조건 함수를 부른다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/strategy/strategy_test.go`:

```go
package strategy

import (
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

func cfg() config.StrategyConfig {
	return config.StrategyConfig{
		IndexMADays: 20, MinTurnover: 10_000_000_000, TurnoverMADays: 20, TurnoverRatioMin: 3.0,
		CloseToHighMin: 0.99, ChangeMin: 0.03, ChangeMax: 0.20, MAShortDays: 20, MALongDays: 60, NewHighDays: 20,
	}
}

// flat 은 n 개의 같은 봉 (종가 close, 고가 close, 거래량 vol).
func flat(n int, close, vol int64) []data.Bar {
	out := make([]data.Bar, n)
	for i := range out {
		out[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close, Volume: vol}
	}
	return out
}

func withLast(bars []data.Bar, last data.Bar) []data.Bar {
	out := append([]data.Bar(nil), bars...)
	last.Date = out[len(out)-1].Date.AddDate(0, 0, 1)
	return append(out, last)
}

func indexFlat(n int, close float64) Market {
	m := Market{}
	for i := 0; i < n; i++ {
		m.IndexBars = append(m.IndexBars, data.IndexBar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close})
	}
	return m
}

func TestMarketFilterBoundary(t *testing.T) {
	c := cfg()
	// 19일 평균 1000, 당일 1000 → 20일 평균 1000, 종가 == 평균 → 차단
	m := indexFlat(20, 1000)
	if MarketFilter(c, m) {
		t.Error("close == ma should fail")
	}
	m.IndexBars[19].Close = 1000.01 // 평균 1000.0005, 종가 > 평균
	if !MarketFilter(c, m) {
		t.Error("close > ma should pass")
	}
	if MarketFilter(c, indexFlat(19, 1000)) {
		t.Error("fewer than index_ma_days bars should fail")
	}
}

func TestLiquidityBoundary(t *testing.T) {
	c := cfg()
	base := flat(20, 10_000, 100_000) // 직전 20일 거래대금 10억
	// 당일 거래대금 = 30억 = 3배 정확히, 그러나 100억 미만 → 실패
	if Liquidity(c, withLast(base, data.Bar{Close: 10_000, High: 10_000, Volume: 300_000})) {
		t.Error("below min_turnover should fail")
	}
	big := flat(20, 10_000, 500_000) // 직전 평균 50억
	// 당일 150억 = 정확히 3배, 100억 이상 → 통과
	if !Liquidity(c, withLast(big, data.Bar{Close: 10_000, High: 10_000, Volume: 1_500_000})) {
		t.Error("exactly 3x and >= min_turnover should pass")
	}
	if Liquidity(c, withLast(big, data.Bar{Close: 10_000, High: 10_000, Volume: 1_499_999})) {
		t.Error("just under 3x should fail")
	}
	if Liquidity(c, flat(20, 10_000, 1_500_000)) { // 직전 봉 19개뿐
		t.Error("fewer than turnover_ma_days+1 bars should fail")
	}
}

func TestCloseStrengthBoundary(t *testing.T) {
	c := cfg()
	if !CloseStrength(c, []data.Bar{{Close: 9_900, High: 10_000}}) {
		t.Error("exactly 0.99 should pass")
	}
	if CloseStrength(c, []data.Bar{{Close: 9_899, High: 10_000}}) {
		t.Error("below 0.99 should fail")
	}
	if CloseStrength(c, nil) {
		t.Error("no bars should fail")
	}
}

func TestChangeBoundary(t *testing.T) {
	c := cfg()
	prev := flat(1, 10_000, 1)
	for _, tc := range []struct {
		close int64
		want  bool
	}{{10_300, true}, {10_299, false}, {11_999, true}, {12_000, false}} {
		if got := Change(c, withLast(prev, data.Bar{Close: tc.close})); got != tc.want {
			t.Errorf("close %d: Change = %v, want %v", tc.close, got, tc.want)
		}
	}
	if Change(c, prev) {
		t.Error("single bar should fail")
	}
}

func TestTrendBoundary(t *testing.T) {
	c := cfg()
	// 59개 1000, 당일 1000 → 20일선 == 60일선 == 종가 → 실패
	if Trend(c, flat(60, 1000, 1)) {
		t.Error("all equal should fail")
	}
	// 40개 900 + 19개 1000 + 당일 1001: 20일선 (19000+1001)/20=1000.05, 60일선 (36000+19000+1001)/60=933.35 → 통과
	bars := append(flat(40, 900, 1), flat(19, 1000, 1)...)
	if !Trend(c, withLast(bars, data.Bar{Close: 1001})) {
		t.Error("close > ma20 > ma60 should pass")
	}
	// 당일 1000: 20일선 = 1000 == 종가 → 실패
	if Trend(c, withLast(bars, data.Bar{Close: 1000})) {
		t.Error("close == ma20 should fail")
	}
	if Trend(c, flat(59, 1000, 1)) {
		t.Error("fewer than ma_long_days bars should fail")
	}
}

func TestNewHighBoundary(t *testing.T) {
	c := cfg()
	prev := flat(20, 1000, 1)
	prev[5].High = 1500
	if !NewHigh(c, withLast(prev, data.Bar{Close: 1500, High: 1600})) {
		t.Error("close == prior 20d high should pass")
	}
	if NewHigh(c, withLast(prev, data.Bar{Close: 1499, High: 1600})) {
		t.Error("close below prior high should fail")
	}
	if NewHigh(c, withLast(flat(19, 1000, 1), data.Bar{Close: 2000})) {
		t.Error("fewer than new_high_days prior bars should fail")
	}
}

func TestEntryRequiresAll(t *testing.T) {
	c := cfg()
	// 통과하는 케이스를 만든다: 직전 60일 900/거래량 100만(거래대금 9억), 당일 종가 1000·고가 1000·거래량 1500만(150억) → 총 61개
	prev := flat(60, 900, 1_000_000)
	last := data.Bar{Open: 950, High: 1000, Low: 940, Close: 1000, Volume: 15_000_000}
	bars := withLast(prev, last)
	m := indexFlat(20, 1000)
	m.IndexBars[19].Close = 1001
	if !Entry(c, m, bars) {
		t.Fatalf("expected entry: mf=%v liq=%v cs=%v ch=%v tr=%v nh=%v", MarketFilter(c, m), Liquidity(c, bars), CloseStrength(c, bars), Change(c, bars), Trend(c, bars), NewHigh(c, bars))
	}
	// 조건을 하나씩 깨뜨린다
	bad := indexFlat(20, 1000)
	if Entry(c, bad, bars) {
		t.Error("market filter off should block")
	}
	weak := withLast(prev, data.Bar{Open: 950, High: 1020, Low: 940, Close: 1000, Volume: 15_000_000}) // 1000/1020 < 0.99
	if Entry(c, m, weak) {
		t.Error("close strength off should block")
	}
	if Entry(c, m, withLast(flat(60, 900, 1_000_000), data.Bar{High: 1000, Close: 1000, Volume: 9_999_999})) { // 99.99억
		t.Error("liquidity off should block")
	}
	if Entry(c, m, bars[1:]) { // 60개: ma_long_days+1 미만
		t.Error("too few bars should block")
	}
}

func TestScore(t *testing.T) {
	c := cfg()
	bars := withLast(flat(20, 1000, 1000), data.Bar{Close: 1000, Volume: 3000})
	if s := Score(c, bars); s < 2.999 || s > 3.001 {
		t.Errorf("score = %v, want 3", s)
	}
	if Score(c, flat(5, 1000, 1)) != 0 {
		t.Error("too few bars should score 0")
	}
	_ = time.Now
}
```
(마지막 `_ = time.Now` 는 넣지 말고 `time` import 도 빼라 — 사용하지 않는다.)

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/strategy/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: Market`)

- [ ] **Step 3: 구현**

`internal/strategy/strategy.go`:

```go
// Package strategy 는 종가 베팅 전략의 진입 조건(설계 8.2절)을 순수 함수로 제공한다.
// 상태를 갖지 않고 넘겨받은 봉만 본다. 호출자가 당일까지의 봉만 넘겨 미래를 보지 않게 한다.
package strategy

import (
	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

// Market 은 종목이 속한 시장의 지수 일봉. 종목 bars 와 같은 규칙으로 당일까지만 담긴다.
type Market struct {
	IndexBars []data.IndexBar
}

// Turnover 는 봉의 거래대금 근사(종가 × 거래량).
func Turnover(b data.Bar) int64 { return b.Close * b.Volume }

// Entry 는 8.2절 진입 조건 8개를 모두 검사한다. bars 는 과거→현재, 마지막이 당일.
func Entry(cfg config.StrategyConfig, mkt Market, bars []data.Bar) bool {
	return MarketFilter(cfg, mkt) &&
		len(bars) >= cfg.MALongDays+1 &&
		Liquidity(cfg, bars) &&
		CloseStrength(cfg, bars) &&
		Change(cfg, bars) &&
		Trend(cfg, bars) &&
		NewHigh(cfg, bars)
}

// MarketFilter (조건 1): 지수 봉이 index_ma_days 개 이상이고 당일 종가 > index_ma_days 일 평균(당일 포함).
func MarketFilter(cfg config.StrategyConfig, mkt Market) bool {
	n := cfg.IndexMADays
	b := mkt.IndexBars
	if n <= 0 || len(b) < n {
		return false
	}
	sum := 0.0
	for _, x := range b[len(b)-n:] {
		sum += x.Close
	}
	return b[len(b)-1].Close > sum/float64(n)
}

// Liquidity (조건 3·4): 당일 거래대금 ≥ min_turnover 이고 ≥ 직전 turnover_ma_days 일 평균(당일 제외) × ratio.
func Liquidity(cfg config.StrategyConfig, bars []data.Bar) bool {
	k := cfg.TurnoverMADays
	if k <= 0 || len(bars) < k+1 {
		return false
	}
	today := float64(Turnover(bars[len(bars)-1]))
	if today < float64(cfg.MinTurnover) {
		return false
	}
	return today >= avgTurnover(bars[len(bars)-1-k:len(bars)-1])*cfg.TurnoverRatioMin
}

// CloseStrength (조건 5): 종가 ≥ 고가 × close_to_high_min.
func CloseStrength(cfg config.StrategyConfig, bars []data.Bar) bool {
	if len(bars) == 0 {
		return false
	}
	b := bars[len(bars)-1]
	return float64(b.Close) >= float64(b.High)*cfg.CloseToHighMin
}

// Change (조건 6): change_min ≤ 종가/전일 종가 − 1 < change_max.
func Change(cfg config.StrategyConfig, bars []data.Bar) bool {
	if len(bars) < 2 || bars[len(bars)-2].Close <= 0 {
		return false
	}
	// 비율(종가/전일−1)로 비교하면 1.2−1 = 0.1999… 같은 부동소수 오차로 경계가 어긋난다.
	// screener 의 문턱값과 같은 식(종가 vs 전일×(1+r))으로 비교한다.
	c, p := float64(bars[len(bars)-1].Close), float64(bars[len(bars)-2].Close)
	return c >= p*(1+cfg.ChangeMin) && c < p*(1+cfg.ChangeMax)
}

// Trend (조건 7): 종가 > 단기 MA > 장기 MA (당일 포함). 봉이 ma_long_days 개 미만이면 false.
func Trend(cfg config.StrategyConfig, bars []data.Bar) bool {
	n, m := cfg.MAShortDays, cfg.MALongDays
	if n <= 0 || m <= n || len(bars) < m {
		return false
	}
	short, long := maClose(bars, n), maClose(bars, m)
	return float64(bars[len(bars)-1].Close) > short && short > long
}

// NewHigh (조건 8): 종가 ≥ 직전 new_high_days 일(당일 제외) 고가 최댓값.
func NewHigh(cfg config.StrategyConfig, bars []data.Bar) bool {
	k := cfg.NewHighDays
	if k <= 0 || len(bars) < k+1 {
		return false
	}
	return bars[len(bars)-1].Close >= maxHigh(bars[len(bars)-1-k:len(bars)-1])
}

// Score 는 후보 우선순위: 당일 거래대금 / 직전 turnover_ma_days 일 평균. 봉이 부족하면 0.
func Score(cfg config.StrategyConfig, bars []data.Bar) float64 {
	k := cfg.TurnoverMADays
	if k <= 0 || len(bars) < k+1 {
		return 0
	}
	avg := avgTurnover(bars[len(bars)-1-k : len(bars)-1])
	if avg == 0 {
		return 0
	}
	return float64(Turnover(bars[len(bars)-1])) / avg
}

// maClose 는 마지막 n 개 종가의 평균.
func maClose(bars []data.Bar, n int) float64 {
	sum := int64(0)
	for _, b := range bars[len(bars)-n:] {
		sum += b.Close
	}
	return float64(sum) / float64(n)
}

func avgTurnover(bars []data.Bar) float64 {
	if len(bars) == 0 {
		return 0
	}
	sum := int64(0)
	for _, b := range bars {
		sum += Turnover(b)
	}
	return float64(sum) / float64(len(bars))
}

func maxHigh(bars []data.Bar) int64 {
	var h int64
	for _, b := range bars {
		if b.High > h {
			h = b.High
		}
	}
	return h
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./internal/strategy/ && go test ./internal/strategy/ -v -count=1 2>&1 | tail -12`
Expected: 8개 테스트 PASS. `TestEntryRequiresAll` 의 첫 단언이 실패하면 각 조건 값이 출력되니 그걸로 봉 구성을 점검하되 조건 함수의 경계는 바꾸지 말 것.

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/strategy
git commit -m "feat(strategy): 종가 베팅 진입 조건 함수"
```

---

### Task 2: 관심종목 선별 (internal/screener)

**Files:**
- Create: `internal/screener/screener.go`, `internal/screener/screener_test.go`

**Interfaces:**
- Consumes: `strategy.*`, `data.Store`(`ListSymbols`, `LoadBars`, `LastIndexBarDate`, `LoadIndexBars`), `config.Config`.
- Produces: `screener.WatchItem{Code, Name, Market string; PrevClose, MinClose int64; MinChangePct float64; MinTurnover, MinVolume int64; Thresholds map[string]int64}`, `screener.Compute(cfg config.StrategyConfig, sym data.Symbol, bars []data.Bar) (WatchItem, bool)`, `screener.Result{AsOf time.Time; Items []WatchItem; Filter map[string]string}` (`Filter[market]` = `진입가능`/`차단`/`알 수 없음`), `screener.Run(ctx, cfg *config.Config, store data.Store) (Result, error)`, `screener.FilterStatus(cfg, bars []data.IndexBar) string`. Task 3 이 `Run`·`Result` 를 쓴다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/screener/screener_test.go`:

```go
package screener

import (
	"context"
	"math"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/strategy"
)

func cfg() config.StrategyConfig {
	return config.StrategyConfig{
		IndexMADays: 20, MinTurnover: 10_000_000_000, TurnoverMADays: 20, TurnoverRatioMin: 3.0,
		CloseToHighMin: 0.99, ChangeMin: 0.03, ChangeMax: 0.20, MAShortDays: 20, MALongDays: 60, NewHighDays: 20,
	}
}

// randomBars 는 무작위 걸음 봉 n 개. 가격은 1,000~100,000 원, 거래량 10만~200만.
func randomBars(r *rand.Rand, n int) []data.Bar {
	out := make([]data.Bar, n)
	price := int64(1000 + r.Intn(50_000))
	for i := range out {
		price += int64(r.Intn(2001) - 1000)
		if price < 1000 {
			price = 1000
		}
		high := price + int64(r.Intn(500))
		low := price - int64(r.Intn(500))
		if low < 1 {
			low = 1
		}
		out[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: price, High: high, Low: low, Close: price, Volume: int64(100_000 + r.Intn(1_900_000))}
	}
	return out
}

func sym() data.Symbol { return data.Symbol{Code: "000001", Name: "테스트", Market: "kospi"} }

// TestThresholdsMatchStrategy 는 9.3절: C = C_min 이면 가격 조건(6·7·8)과 유동성이 통과하고, C = C_min−1 이면 가격 조건 중 하나가 실패한다.
func TestThresholdsMatchStrategy(t *testing.T) {
	c := cfg()
	r := rand.New(rand.NewSource(42))
	feasible := 0
	for i := 0; i < 500; i++ {
		prev := randomBars(r, 60+r.Intn(30))
		item, ok := Compute(c, sym(), prev)
		if !ok {
			continue
		}
		feasible++
		next := prev[len(prev)-1].Date.AddDate(0, 0, 1)
		pass := append(append([]data.Bar(nil), prev...), data.Bar{Date: next, Open: item.MinClose, High: item.MinClose, Low: item.MinClose, Close: item.MinClose, Volume: item.MinVolume})
		if !strategy.Change(c, pass) || !strategy.Trend(c, pass) || !strategy.NewHigh(c, pass) || !strategy.Liquidity(c, pass) || !strategy.CloseStrength(c, pass) {
			t.Fatalf("case %d: C=C_min should pass: change=%v trend=%v newhigh=%v liq=%v item=%+v", i,
				strategy.Change(c, pass), strategy.Trend(c, pass), strategy.NewHigh(c, pass), strategy.Liquidity(c, pass), item)
		}
		fail := append(append([]data.Bar(nil), prev...), data.Bar{Date: next, Open: item.MinClose - 1, High: item.MinClose - 1, Low: item.MinClose - 1, Close: item.MinClose - 1, Volume: item.MinVolume * 10})
		if strategy.Change(c, fail) && strategy.Trend(c, fail) && strategy.NewHigh(c, fail) {
			t.Fatalf("case %d: C=C_min-1 should fail a price condition: item=%+v", i, item)
		}
		if item.PrevClose != prev[len(prev)-1].Close || item.MinChangePct != float64(item.MinClose)/float64(item.PrevClose)-1 {
			t.Fatalf("case %d: derived fields wrong: %+v", i, item)
		}
	}
	if feasible < 20 {
		t.Fatalf("too few feasible cases to trust the test: %d", feasible)
	}
}

func TestComputeRejectsShortHistoryAndInfeasible(t *testing.T) {
	c := cfg()
	r := rand.New(rand.NewSource(7))
	if _, ok := Compute(c, sym(), randomBars(r, 59)); ok {
		t.Error("fewer than ma_long_days bars must be rejected")
	}
	// 직전 20일 고가가 전일 종가의 1.5배면 상승률 상한(20%) 안에서 신고가를 못 넘긴다 → 실현 불가
	bars := randomBars(r, 60)
	last := bars[len(bars)-1].Close
	bars[len(bars)-5].High = last * 3 / 2
	if _, ok := Compute(c, sym(), bars); ok {
		t.Error("infeasible (new high beyond change_max) must be rejected")
	}
}

func TestComputeThresholdValues(t *testing.T) {
	c := cfg()
	// 60개 모두 종가 10,000·고가 10,000·거래량 100만(거래대금 100억). 단기·장기선 하한 = 10,001, 상승률 하한 = 10,300, 신고가 = 10,000 → C_min 10,300
	bars := make([]data.Bar, 60)
	for i := range bars {
		bars[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: 10_000, High: 10_000, Low: 10_000, Close: 10_000, Volume: 1_000_000}
	}
	item, ok := Compute(c, sym(), bars)
	if !ok {
		t.Fatal("expected feasible")
	}
	if item.MinClose != 10_300 || item.Thresholds["change_min"] != 10_300 || item.Thresholds["new_high"] != 10_000 || item.Thresholds["ma_short"] != 10_001 || item.Thresholds["ma_long"] != 10_001 {
		t.Errorf("thresholds = %+v", item)
	}
	// T = 100억, ratio 3 → T_req 300억, MinVolume = ceil(300억 / 10,300) = 2,912,622
	if item.MinTurnover != 30_000_000_000 || item.MinVolume != int64(math.Ceil(30_000_000_000.0/10_300)) {
		t.Errorf("turnover thresholds = %+v", item)
	}
	if math.Abs(item.MinChangePct-0.03) > 1e-9 {
		t.Errorf("MinChangePct = %v", item.MinChangePct)
	}
}

func TestFilterStatus(t *testing.T) {
	c := cfg()
	mk := func(n int, last float64) []data.IndexBar {
		out := make([]data.IndexBar, n)
		for i := range out {
			out[i] = data.IndexBar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Close: 1000}
		}
		if n > 0 {
			out[n-1].Close = last
		}
		return out
	}
	if s := FilterStatus(c, mk(20, 1001)); s != "진입가능" {
		t.Errorf("above ma: %q", s)
	}
	if s := FilterStatus(c, mk(20, 1000)); s != "차단" {
		t.Errorf("on ma: %q", s)
	}
	if s := FilterStatus(c, mk(19, 1001)); s != "알 수 없음" {
		t.Errorf("short: %q", s)
	}
}

func TestRunSortsAndUsesIndexDate(t *testing.T) {
	ctx := context.Background()
	store, err := data.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := &config.Config{Universe: config.UniverseConfig{Markets: []string{"kospi", "kosdaq"}}, Strategy: cfg()}
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "B", Name: "비", Market: "kospi"}, {Code: "A", Name: "에이", Market: "kosdaq"}, {Code: "S", Name: "짧음", Market: "kospi"}})
	flat := func(n int, close, vol int64, extra int) []data.Bar {
		out := make([]data.Bar, n+extra)
		for i := range out {
			out[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close, Volume: vol}
		}
		return out
	}
	// B: 60개 + 지수 이후 날짜 1개(잘려야 함, 값이 다름) ; A: 60개, 마지막 종가 더 낮은 문턱 ; S: 30개
	bB := flat(60, 10_000, 1_000_000, 1)
	bB[60].Close = 99_999 // 지수 마지막 날짜 뒤 → 무시되어야 함
	store.UpsertBars(ctx, "B", bB)
	store.UpsertBars(ctx, "A", flat(60, 10_000, 1_000_000, 0))
	store.UpsertBars(ctx, "S", flat(30, 10_000, 1_000_000, 0))
	idx := make([]data.IndexBar, 60)
	for i := range idx {
		idx[i] = data.IndexBar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Close: 1000}
	}
	idx[59].Close = 1001
	store.UpsertIndexBars(ctx, "kospi", idx)
	store.UpsertIndexBars(ctx, "kosdaq", idx[:10])

	res, err := Run(ctx, c, store)
	if err != nil {
		t.Fatal(err)
	}
	if !res.AsOf.Equal(data.Date(2026, 1, 1).AddDate(0, 0, 59)) {
		t.Errorf("AsOf = %v", res.AsOf)
	}
	if len(res.Items) != 2 || res.Items[0].Code != "A" || res.Items[1].Code != "B" {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[1].PrevClose != 10_000 {
		t.Errorf("B should ignore bars after index date: %+v", res.Items[1])
	}
	if res.Filter["kospi"] != "진입가능" || res.Filter["kosdaq"] != "알 수 없음" {
		t.Errorf("filter = %v", res.Filter)
	}
}

func TestRunWithoutIndexReturnsEmpty(t *testing.T) {
	store, _ := data.Open(filepath.Join(t.TempDir(), "t.db"))
	defer store.Close()
	c := &config.Config{Universe: config.UniverseConfig{Markets: []string{"kospi"}}, Strategy: cfg()}
	res, err := Run(context.Background(), c, store)
	if err != nil || len(res.Items) != 0 || !res.AsOf.IsZero() {
		t.Errorf("empty store: %+v %v", res, err)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/screener/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: Compute`)

- [ ] **Step 3: 구현**

`internal/screener/screener.go`:

```go
// Package screener 는 전일까지의 일봉으로 "오늘 종가가 얼마 이상이면 진입 조건을 통과하는가"(설계 9절)를 종목마다 계산한다.
// 네트워크를 쓰지 않는다.
package screener

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/strategy"
)

// WatchItem 은 관심 종목 한 줄.
type WatchItem struct {
	Code, Name, Market string
	PrevClose          int64
	MinClose           int64            // C_min: 오늘 종가 하한
	MinChangePct       float64          // C_min / P − 1
	MinTurnover        int64            // T_req
	MinVolume          int64            // T_req / C_min (올림)
	Thresholds         map[string]int64 // 조건별 하한: change_min, new_high, ma_short, ma_long
}

// Compute 는 전일까지의 봉(과거→현재)으로 문턱값을 계산한다. 봉이 ma_long_days 개 미만이거나 실현 불가능하면 false.
func Compute(cfg config.StrategyConfig, sym data.Symbol, bars []data.Bar) (WatchItem, bool) {
	n, m, k, h := cfg.MAShortDays, cfg.MALongDays, cfg.TurnoverMADays, cfg.NewHighDays
	if n <= 1 || m <= n || k <= 0 || h <= 0 || len(bars) < m || len(bars) < k || len(bars) < h {
		return WatchItem{}, false
	}
	p := bars[len(bars)-1].Close
	if p <= 0 {
		return WatchItem{}, false
	}
	sumN, sumM := sumClose(bars, n-1), sumClose(bars, m-1)

	th := map[string]int64{
		"change_min": ceilPrice(float64(p) * (1 + cfg.ChangeMin)),
		"new_high":   maxHigh(bars[len(bars)-h:]),
		"ma_short":   floorDiv(sumN, int64(n-1)) + 1,
		"ma_long":    max(0, floorDiv(int64(n)*sumM-int64(m)*sumN, int64(m-n))+1),
	}
	cmin := int64(0)
	for _, v := range th {
		if v > cmin {
			cmin = v
		}
	}
	if float64(cmin) >= float64(p)*(1+cfg.ChangeMax) {
		return WatchItem{}, false
	}
	avgT := 0.0
	for _, b := range bars[len(bars)-k:] {
		avgT += float64(strategy.Turnover(b))
	}
	avgT /= float64(k)
	treq := int64(math.Ceil(math.Max(float64(cfg.MinTurnover), avgT*cfg.TurnoverRatioMin)))
	return WatchItem{
		Code: sym.Code, Name: sym.Name, Market: sym.Market,
		PrevClose:    p,
		MinClose:     cmin,
		MinChangePct: float64(cmin)/float64(p) - 1,
		MinTurnover:  treq,
		MinVolume:    int64(math.Ceil(float64(treq) / float64(cmin))),
		Thresholds:   th,
	}, true
}

// FilterStatus 는 시장 필터 참고 정보: 진입가능 / 차단 / 알 수 없음(봉 부족).
func FilterStatus(cfg config.StrategyConfig, bars []data.IndexBar) string {
	if len(bars) < cfg.IndexMADays || cfg.IndexMADays <= 0 {
		return "알 수 없음"
	}
	if strategy.MarketFilter(cfg, strategy.Market{IndexBars: bars}) {
		return "진입가능"
	}
	return "차단"
}

// Result 는 Run 의 출력.
type Result struct {
	AsOf   time.Time         // 기준일 = 코스피 지수 마지막 봉 날짜. 지수가 없으면 zero
	Items  []WatchItem       // MinChangePct 오름차순, 같으면 코드 오름차순
	Filter map[string]string // 시장별 필터 상태
}

// Run 은 저장소의 전 종목에 Compute 를 적용한다. 기준일 이후의 봉은 무시한다.
func Run(ctx context.Context, cfg *config.Config, store data.Store) (Result, error) {
	res := Result{Filter: map[string]string{}}
	asOf, ok, err := store.LastIndexBarDate(ctx, "kospi")
	if err != nil {
		return res, err
	}
	if !ok {
		for _, mkt := range cfg.Universe.Markets {
			res.Filter[mkt] = "알 수 없음"
		}
		return res, nil
	}
	res.AsOf = asOf
	from := asOf.AddDate(0, 0, -400) // 60거래일 + 여유
	for _, mkt := range cfg.Universe.Markets {
		ib, err := store.LoadIndexBars(ctx, mkt, from, asOf)
		if err != nil {
			return res, err
		}
		res.Filter[mkt] = FilterStatus(cfg.Strategy, ib)
	}
	syms, err := store.ListSymbols(ctx)
	if err != nil {
		return res, err
	}
	for _, s := range syms {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		bars, err := store.LoadBars(ctx, s.Code, from, asOf)
		if err != nil {
			return res, err
		}
		if item, ok := Compute(cfg.Strategy, s, bars); ok {
			res.Items = append(res.Items, item)
		}
	}
	sort.Slice(res.Items, func(i, j int) bool {
		if res.Items[i].MinChangePct != res.Items[j].MinChangePct {
			return res.Items[i].MinChangePct < res.Items[j].MinChangePct
		}
		return res.Items[i].Code < res.Items[j].Code
	})
	return res, nil
}

func sumClose(bars []data.Bar, n int) int64 {
	s := int64(0)
	for _, b := range bars[len(bars)-n:] {
		s += b.Close
	}
	return s
}

func maxHigh(bars []data.Bar) int64 {
	var h int64
	for _, b := range bars {
		if b.High > h {
			h = b.High
		}
	}
	return h
}

// ceilPrice 는 부동소수 오차(예: 10000×1.03 = 10300.000000000002)로 1원 튀지 않게 작은 값을 뺀 뒤 올린다.
func ceilPrice(v float64) int64 {
	return int64(math.Ceil(v - 1e-6))
}

// floorDiv 는 음수도 내림 나눗셈.
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./internal/screener/ && go test ./internal/screener/ -v -count=1 2>&1 | tail -12`
Expected: 6개 PASS. `TestThresholdsMatchStrategy` 가 실패하면 대수식과 조건 함수 경계 중 하나가 어긋난 것 — 실패 케이스의 `item` 과 조건 값을 보고서에 적고, 스펙 9.1 표를 기준으로 어느 쪽이 틀렸는지 판단한다. 기대값을 느슨하게 만들지 말 것.

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/screener
git commit -m "feat(screener): 관심종목 문턱값 계산과 선별"
```

---

### Task 3: `trader watch`, TUI 관심종목 실데이터, 하단 전일 지수

**Files:**
- Modify: `cmd/trader/main.go`, `internal/tui/types.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/autofetch.go`, `internal/tui/run.go`, `internal/tui/fake.go`, `internal/tui/model_test.go`, `internal/tui/view_test.go`
- Create: `internal/tui/screen.go`, `internal/tui/screen_test.go`

**Interfaces:**
- Consumes: `screener.Run`, `screener.Result`, `screener.WatchItem`, `data.Store.LoadIndexBars`.
- Produces: `tui.IndexPrevCloseMsg{Market string; Close float64}`, 비공개 `watchMsgFrom(screener.Result) WatchMsg`, `runScreen(ctx, cfg, store, logger, send)`; `runFetchOnce` 에 `onDone func()` 인자 추가. 8단계는 `runFetchOnce` 의 자리에 잔고 폴링을 덧붙인다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/screen_test.go`:

```go
package tui

import (
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/screener"
)

func TestWatchMsgFromResult(t *testing.T) {
	res := screener.Result{
		AsOf:   data.Date(2026, 9, 16),
		Filter: map[string]string{"kospi": "진입가능", "kosdaq": "차단"},
		Items: []screener.WatchItem{
			{Code: "005930", Name: "삼성전자", Market: "kospi", PrevClose: 71200, MinClose: 73400, MinChangePct: 0.031, MinVolume: 2140000},
			{Code: "247540", Name: "에코프로비엠", Market: "kosdaq", PrevClose: 98500, MinClose: 102300, MinChangePct: 0.039, MinVolume: 910000},
		},
	}
	msg := watchMsgFrom(res)
	if msg.AsOf != "09-16" || msg.Filter.Kospi != "진입가능" || msg.Filter.Kosdaq != "차단" {
		t.Errorf("header = %+v", msg)
	}
	if len(msg.Rows) != 2 || msg.Rows[0].Name != "삼성전자" || msg.Rows[0].Market != "코스피" || msg.Rows[1].Market != "코스닥" || msg.Rows[0].MinVolume != 2140000 {
		t.Errorf("rows = %+v", msg.Rows)
	}
	empty := watchMsgFrom(screener.Result{Filter: map[string]string{"kospi": "알 수 없음", "kosdaq": "알 수 없음"}})
	if empty.AsOf != "" || len(empty.Rows) != 0 || empty.Filter.Kospi != "알 수 없음" {
		t.Errorf("empty = %+v", empty)
	}
	_ = time.Now
}
```
(`_ = time.Now` 와 `time` import 는 넣지 않는다.)

`internal/tui/model_test.go` 끝에:

```go
func TestIndexPrevCloseShownWhenDisconnected(t *testing.T) {
	m := sized(t)
	m = send(m, IndexPrevCloseMsg{Market: "kospi", Close: 2712.4})
	if !strings.Contains(m.View(), "코스피 2,712.40 (전일)") {
		t.Errorf("prev close should show when no live quote:\n%s", m.View())
	}
	m = send(m, ConnectedMsg{})
	if !strings.Contains(m.View(), "코스피 2,712.40 (전일)") {
		t.Errorf("prev close should still show while connected but no tick:\n%s", m.View())
	}
	m = send(m, IndexMsg{Market: "kospi", Value: 2720, ChangePct: 0.003})
	if strings.Contains(m.View(), "(전일)") || !strings.Contains(m.View(), "2,720.00") {
		t.Errorf("live quote should replace prev close:\n%s", m.View())
	}
	m = send(m, DisconnectedMsg{})
	if !strings.Contains(m.View(), "코스피 2,712.40 (전일)") {
		t.Errorf("after disconnect prev close should return:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "코스닥 미연결") {
		t.Errorf("kosdaq without prev close stays 미연결:\n%s", m.View())
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: watchMsgFrom`, `IndexPrevCloseMsg`)

- [ ] **Step 3: types.go / model.go / view.go**

`types.go` 의 `IndexMsg` 뒤에:

```go
// IndexPrevCloseMsg 는 저장소의 지수 전일 종가. 실시간 값이 없을 때 "(전일)" 로 보인다.
type IndexPrevCloseMsg struct {
	Market string
	Close  float64
}
```

`model.go`: `indexQuote` 에 `PrevClose float64` 필드 추가. `Update` 에 케이스:

```go
	case IndexPrevCloseMsg:
		switch msg.Market {
		case "kospi":
			m.kospi.PrevClose = msg.Close
		case "kosdaq":
			m.kosdaq.PrevClose = msg.Close
		}
```
`IndexMsg` 케이스는 `q := indexQuote{…, Connected: true}` 로 덮어쓰므로 `PrevClose` 를 보존하도록 `q.PrevClose = m.kospi.PrevClose`(각 시장) 를 넣거나, 필드만 갱신하는 형태로 바꾼다. `DisconnectedMsg` 는 `Connected=false` 만 바꾸고 `PrevClose` 는 남긴다 (현재 코드가 그렇다 — 확인).

`view.go` `fmtIndex`:

```go
func fmtIndex(q indexQuote, linkOK bool) string {
	if !q.Connected {
		if q.PrevClose > 0 {
			return commaF(q.PrevClose) + " (전일)"
		}
		if linkOK {
			return "연결됨 · 장외"
		}
		return "미연결"
	}
	…
```

- [ ] **Step 4: screen.go**

`internal/tui/screen.go`:

```go
package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/screener"
)

var marketNames = map[string]string{"kospi": "코스피", "kosdaq": "코스닥"}

// watchMsgFrom 은 선별 결과를 화면 메시지로 바꾼다.
func watchMsgFrom(res screener.Result) WatchMsg {
	msg := WatchMsg{Filter: MarketFilter{Kospi: res.Filter["kospi"], Kosdaq: res.Filter["kosdaq"]}}
	if !res.AsOf.IsZero() {
		msg.AsOf = res.AsOf.In(data.KST).Format("01-02")
	}
	for _, it := range res.Items {
		name := marketNames[it.Market]
		if name == "" {
			name = it.Market
		}
		msg.Rows = append(msg.Rows, WatchRow{Name: it.Name, Market: name, PrevClose: it.PrevClose, MinClose: it.MinClose, MinChangePct: it.MinChangePct, MinVolume: it.MinVolume})
	}
	return msg
}

// runScreen 은 선별을 한 번 돌려 관심종목과 지수 전일 종가를 화면에 보낸다. 실패는 로그만.
func runScreen(ctx context.Context, cfg *config.Config, store data.Store, logger *slog.Logger, send func(tea.Msg)) {
	res, err := screener.Run(ctx, cfg, store)
	if err != nil {
		logger.Error("관심종목 계산 실패", "err", err)
		return
	}
	logger.Info("관심종목 계산", "asof", res.AsOf.Format("2006-01-02"), "items", len(res.Items), "kospi", res.Filter["kospi"], "kosdaq", res.Filter["kosdaq"])
	send(watchMsgFrom(res))
	if res.AsOf.IsZero() {
		return
	}
	for _, mkt := range cfg.Universe.Markets {
		bars, err := store.LoadIndexBars(ctx, mkt, res.AsOf, res.AsOf)
		if err != nil || len(bars) == 0 {
			continue
		}
		send(IndexPrevCloseMsg{Market: mkt, Close: bars[len(bars)-1].Close})
	}
}
```

- [ ] **Step 5: autofetch.go / run.go / fake.go**

`autofetch.go`: `runFetchOnce` 시그니처에 마지막 인자 `onDone func()` 추가; 함수 끝(로그 뒤)에 `if onDone != nil { onDone() }`. `Skipped` 든 완료든 취소가 아니면 호출한다 (취소·오류면 호출하지 않음).

`run.go`: 저장소를 자동 수집 블록 밖으로 끌어올려 앱키 유무와 무관하게 연다:

```go
	store, storeErr := data.Open(cfg.DBPath)
	if storeErr != nil {
		logger.With("kind", "지수").Error("DB 열기 실패 — 관심종목·자동 수집 비활성", "err", storeErr)
	} else {
		defer store.Close()
		screenLog := logger.With("kind", "지수")
		go runScreen(ctx, cfg, store, screenLog, p.Send)
		// 자동 수집 (실전 키 필요)
		fetchLog := logger.With("kind", "수집")
		if err := cfg.RequireMarketKey(); err != nil {
			fetchLog.Warn("자동 수집 비활성: " + err.Error())
		} else {
			client := kis.New(…)
			… (기존 stale/next 정의)
			go fetchLoop(ctx, time.Now, stale, next, func(c context.Context) {
				runFetchOnce(c, cfg, store, client, fetchLog, p.Send, func() { runScreen(c, cfg, store, screenLog, p.Send) })
			})
		}
	}
```
기존 `data.Open` 이 자동 수집 블록 안에 있던 것을 위 구조로 옮긴다. `defer store.Close()` 는 함수 끝 `cancel()` 뒤에 실행되므로 그대로 안전.

`fake.go`: `WatchMsg{…}` 블록 삭제, 주석을 "8단계(보유종목)에서 실데이터로 바꾸며 삭제한다" 로.

- [ ] **Step 6: `trader watch`**

`cmd/trader/main.go`: `case "watch": return runWatch(ctx, cfg)` 로 바꾸고 추가:

```go
// runWatch 는 저장소만 읽어 관심종목 표를 출력한다. 네트워크를 쓰지 않는다.
func runWatch(ctx context.Context, cfg *config.Config) error {
	store, err := data.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	res, err := screener.Run(ctx, cfg, store)
	if err != nil {
		return err
	}
	if res.AsOf.IsZero() {
		fmt.Println("지수 봉이 없습니다. 먼저 fetch 를 실행하세요.")
		return nil
	}
	fmt.Printf("기준일 %s  코스피 %s · 코스닥 %s  관심종목 %d개\n", res.AsOf.Format("2006-01-02"), res.Filter["kospi"], res.Filter["kosdaq"], len(res.Items))
	fmt.Printf("%-8s %-16s %-6s %10s %10s %8s %12s\n", "코드", "종목명", "시장", "전일종가", "필요종가", "필요상승", "필요거래량")
	for _, it := range res.Items {
		fmt.Printf("%-8s %-16s %-6s %10d %10d %7.1f%% %12d\n", it.Code, it.Name, it.Market, it.PrevClose, it.MinClose, it.MinChangePct*100, it.MinVolume)
	}
	return nil
}
```
(usage 의 `watch` 줄을 `trader watch           전일 데이터 기준 관심 종목 표 출력` 으로. `%-16s` 는 한글 폭을 못 맞추지만 CLI 는 참고용이라 그대로 둔다 — 주석으로 명시.)

- [ ] **Step 7: 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... -count=1 -race 2>&1 | tail -12`
Expected: 전부 ok. 그리고 실제 DB 가 있으면 `go run ./cmd/trader watch | head -15` 로 표가 나오는지 본다 (수집이 아직 안 끝났으면 일부 종목만; 네트워크 안 씀).

- [ ] **Step 8: 커밋 요청**

```bash
git add cmd internal/tui
git commit -m "feat: trader watch, TUI 관심종목 실데이터·수집 후 재계산, 지수 전일 종가 표시"
```

---

### Task 4: 스펙 갱신과 화면 확인

- [ ] **Step 1: 스펙**

10.1절 관심종목 행: "시작 시 1회" → "시작 시 1회 + 자동 수집 완료 후 재계산 (`screener.Run`)". 하단 행: `(전일)` 표시가 이제 구현됨을 반영 ("7단계부터" 문구 삭제). 12절 strategy·screener 항목이 이 계획의 테스트와 일치하는지 확인하고 문구 맞춤. 4절 CLI 표에 `watch` 가 이미 있음 — 확인.

- [ ] **Step 2: 화면 확인 (사용자, 수집 완료 후)**

1. `go run ./cmd/trader watch | head` — 기준일과 종목 표가 나온다.
2. `go run ./cmd/trader` — 관심종목 패널에 실제 종목이 필요상승 낮은 순으로; 제목 줄에 `(09-17 기준, N개)  코스피 진입가능/차단 · 코스닥 …`; 하단에 실시간 지수 없을 땐 `2,7xx.xx (전일)`.
3. 로그 패널에 `[지수] 관심종목 계산 asof=… items=N`.

- [ ] **Step 3: 커밋 요청**

```bash
git add docs
git commit -m "docs: 7단계 반영"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `gofmt -l .` 비어 있음
- `trader watch` 와 TUI 관심종목 패널이 같은 목록을 보여줌
- 8단계는 `runFetchOnce` 옆에 잔고 폴링을 붙이고, `runScreen` 은 그대로 둔다
