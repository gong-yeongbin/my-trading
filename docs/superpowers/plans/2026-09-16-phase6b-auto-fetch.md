# 6.5단계: TUI 자동 수집 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** TUI 가 켜져 있으면 매일 `fetch.daily_at`(기본 04:00 KST)에 전날 일봉을 스스로 받고, 켤 때 데이터가 오래됐으면 즉시 한 번 받는다. 주말·공휴일엔 지수 2건만 확인하고 종목 호출을 건너뛴다. 진행 상황은 하단 줄과 로그 패널에 보인다.

**Architecture:** `config.Fetch.DailyAt` 추가 → `app` 에 순수 함수 `NextRun`(다음 실행 시각)·`Stale`(따라잡기 판정)과 `FetchOptions.SkipSymbolsIfIndexUnchanged` → `tui/run.go` 가 저장소·한투 클라이언트를 열고 `fetchLoop` 고루틴을 띄워 `RunFetch` 를 돌리며 `FetchStatusMsg` 로 하단 줄을 갱신. 서브커맨드 `trader fetch` 는 수동용으로 그대로.

**Tech Stack:** 표준 라이브러리만. 새 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (4절, 5절 `fetch`, 10.1절 하단, 11절). Task 3 에서 갱신.

**선행:** 6단계 완료 (`app.RunFetch`, `kis`, `data`, `logfile`).

## Global Constraints

- 모듈 경로 `github.com/gong-yeongbin/my-trading`. 새 외부 의존성 없음. 테스트는 표준 `testing`.
- 시각은 전부 KST(`data.KST`). `daily_at` 은 `HH:MM`, 비어 있으면 `04:00`.
- 따라잡기 판정: 코스피 지수 마지막 봉 날짜가 "오늘 이전의 마지막 평일" 보다 이전이면(또는 봉이 없으면) 오래된 것. 공휴일은 판정에 없다 — 그 경우 지수 호출 2건만 낭비된다.
- `SkipSymbolsIfIndexUnchanged` 가 켜져 있고 지수 새 봉이 0건이면 종목 루프 전에 `FetchResult{Skipped: true}` 로 반환.
- TUI 는 로직을 갖지 않는다. 판정은 `app`, TUI 는 `FetchStatusMsg` 를 받아 그린다.
- 한투 앱키가 없으면 스케줄을 잡지 않고 `kind=수집` 경고 로그 한 줄. DB 를 못 열면 마찬가지로 오류 로그 한 줄, TUI 는 계속.
- 수집 고루틴은 TUI 의 자식 ctx 를 쓴다 (`q` 종료 시 함께 취소). 수집 중 취소되면 종목 단위로 저장된 것까지 남는다.
- 한글 폭은 `lipgloss.Width`/`ansi.Truncate`. 색 금지.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/*.json` 읽기 금지. 실서버 호출 금지(테스트). `gofmt -l .` 비어야 함. `go run ./cmd/trader`(인자 없음) 실행 금지.

---

## 파일 구조

```
internal/config/config.go       FetchConfig.DailyAt (수정)
internal/config/config_test.go  (수정)
config.yaml                     fetch.daily_at (수정)
internal/app/schedule.go        NextRun, Stale
internal/app/schedule_test.go
internal/app/fetch.go           SkipSymbolsIfIndexUnchanged, FetchResult.Skipped (수정)
internal/app/fetch_test.go      (수정)
internal/tui/types.go           FetchStatusMsg (수정)
internal/tui/model.go           fetch 상태 (수정)
internal/tui/view.go            하단 줄 수집 진행 (수정)
internal/tui/autofetch.go       fetchLoop, runFetchOnce
internal/tui/autofetch_test.go
internal/tui/run.go             저장소·kis 열기, fetchLoop 배선 (수정)
internal/tui/model_test.go, view_test.go  (수정)
```

---

### Task 1: 설정·스케줄 판정·지수 무변화 건너뛰기 (config, app)

**Files:**
- Modify: `config.yaml`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/app/fetch.go`, `internal/app/fetch_test.go`
- Create: `internal/app/schedule.go`, `internal/app/schedule_test.go`

**Interfaces:**
- Produces: `config.FetchConfig.DailyAt string` (Load 가 비어 있으면 `"04:00"` 으로 채움, `HH:MM` 검증), `app.NextRun(now time.Time, dailyAt string) (time.Time, error)`, `app.Stale(lastIndex time.Time, has bool, now time.Time) bool`, `app.FetchOptions.SkipSymbolsIfIndexUnchanged bool`, `app.FetchResult.Skipped bool`. Task 2 가 전부 쓴다.

- [ ] **Step 1: config 테스트 추가**

`internal/config/config_test.go`:
- `goodYAML` 의 `fetch:` 블록에 `  daily_at: "04:00"` 추가.
- `TestLoadGood` 에 `if cfg.Fetch.DailyAt != "04:00" { t.Errorf(...) }` 추가.
- 새 테스트:

```go
func TestFetchDailyAtDefaultAndValidation(t *testing.T) {
	y := strings.Replace(goodYAML, "  daily_at: \"04:00\"\n", "", 1)
	cfg, err := Load(writeTemp(t, "c.yaml", y))
	if err != nil || cfg.Fetch.DailyAt != "04:00" {
		t.Fatalf("missing daily_at should default to 04:00: %q %v", cfg.Fetch.DailyAt, err)
	}
	for _, bad := range []string{`"4:00"`, `"24:00"`, `"04:60"`, `"abc"`} {
		y := strings.Replace(goodYAML, `daily_at: "04:00"`, "daily_at: "+bad, 1)
		if _, err := Load(writeTemp(t, "c.yaml", y)); err == nil {
			t.Errorf("daily_at %s should fail validation", bad)
		}
	}
}
```

- [ ] **Step 2: config 구현**

`internal/config/config.go`:

```go
type FetchConfig struct {
	StartDate string `yaml:"start_date"`
	DailyAt   string `yaml:"daily_at"` // HH:MM KST. TUI 가 매일 이 시각에 전날 일봉을 받는다. 비어 있으면 04:00
}
```

`Load` 에서 `yaml.Unmarshal` 직후: `if cfg.Fetch.DailyAt == "" { cfg.Fetch.DailyAt = "04:00" }`.
`Validate` 에 추가: `if _, err := time.Parse("15:04", c.Fetch.DailyAt); err != nil || len(c.Fetch.DailyAt) != 5 { errs = append(errs, fmt.Errorf("fetch.daily_at must be HH:MM: %q", c.Fetch.DailyAt)) }`.

`config.yaml` 의 `fetch:` 블록에 `  daily_at: "04:00"          # TUI 자동 수집 시각 (KST). 전날 봉을 새벽에 받는다` 추가.

Run: `go test ./internal/config/ -count=1` → ok.

- [ ] **Step 3: 스케줄 테스트 작성**

`internal/app/schedule_test.go`:

```go
package app

import (
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

func kst(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, data.KST)
}

func TestNextRun(t *testing.T) {
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{kst(2026, 9, 16, 3, 59), kst(2026, 9, 16, 4, 0)},     // 오늘 04:00 전 → 오늘
		{kst(2026, 9, 16, 4, 0), kst(2026, 9, 17, 4, 0)},      // 정각이면 내일
		{kst(2026, 9, 16, 22, 30), kst(2026, 9, 17, 4, 0)},    // 밤 → 내일
		{kst(2026, 12, 31, 23, 0), kst(2027, 1, 1, 4, 0)},     // 연말 → 새해
	}
	for _, tc := range cases {
		got, err := NextRun(tc.now, "04:00")
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("NextRun(%v) = %v, %v; want %v", tc.now, got, err, tc.want)
		}
	}
	// 다른 시간대로 들어와도 KST 기준
	utc := time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC) // KST 16 05:00
	got, _ := NextRun(utc, "04:00")
	if !got.Equal(kst(2026, 9, 17, 4, 0)) {
		t.Errorf("UTC input: %v", got)
	}
	if _, err := NextRun(kst(2026, 9, 16, 0, 0), "4:00"); err == nil {
		t.Error("bad format should error")
	}
}

func TestStale(t *testing.T) {
	cases := []struct {
		name string
		last time.Time
		has  bool
		now  time.Time
		want bool
	}{
		{"no data", time.Time{}, false, kst(2026, 9, 16, 9, 0), true},
		{"tue morning, has mon", data.Date(2026, 9, 14), true, kst(2026, 9, 15, 9, 0), false},
		{"tue morning, has fri", data.Date(2026, 9, 11), true, kst(2026, 9, 15, 9, 0), true},
		{"mon morning, has fri", data.Date(2026, 9, 11), true, kst(2026, 9, 14, 9, 0), false},
		{"sat, has fri", data.Date(2026, 9, 18), true, kst(2026, 9, 19, 12, 0), false},
		{"sun, has thu", data.Date(2026, 9, 17), true, kst(2026, 9, 20, 12, 0), true},
		{"has today already", data.Date(2026, 9, 16), true, kst(2026, 9, 16, 23, 0), false},
	}
	for _, tc := range cases {
		if got := Stale(tc.last, tc.has, tc.now); got != tc.want {
			t.Errorf("%s: Stale = %v, want %v", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 4: 스케줄 구현**

`internal/app/schedule.go`:

```go
package app

import (
	"fmt"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

// NextRun 은 now 이후 처음 오는 dailyAt(HH:MM, KST) 시각. now 가 정확히 그 시각이면 다음 날.
func NextRun(now time.Time, dailyAt string) (time.Time, error) {
	hm, err := time.Parse("15:04", dailyAt)
	if err != nil || len(dailyAt) != 5 {
		return time.Time{}, fmt.Errorf("daily_at must be HH:MM: %q", dailyAt)
	}
	n := now.In(data.KST)
	next := time.Date(n.Year(), n.Month(), n.Day(), hm.Hour(), hm.Minute(), 0, 0, data.KST)
	if !next.After(n) {
		next = next.AddDate(0, 0, 1)
	}
	return next, nil
}

// Stale 은 지수 마지막 봉이 "오늘 이전의 마지막 평일" 보다 오래됐는지. 봉이 없으면 true. 공휴일은 모른다.
func Stale(lastIndex time.Time, has bool, now time.Time) bool {
	if !has {
		return true
	}
	d := now.In(data.KST)
	prev := data.Date(d.Year(), d.Month(), d.Day()).AddDate(0, 0, -1)
	for prev.Weekday() == time.Saturday || prev.Weekday() == time.Sunday {
		prev = prev.AddDate(0, 0, -1)
	}
	return lastIndex.In(data.KST).Before(prev)
}
```

Run: `go test ./internal/app/ -run 'TestNextRun|TestStale' -v -count=1` → PASS.

- [ ] **Step 5: 지수 무변화 건너뛰기 테스트**

`internal/app/fetch_test.go`: `fakeBars` 에 필드 `noIndexBars bool` 을 추가하고 `IndexBars` 에서 `if f.noIndexBars { return nil, nil }` 를 `indexErr` 검사 뒤에 넣는다. 그리고 테스트 추가:

```go
func TestRunFetchSkipsSymbolsWhenIndexUnchanged(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "005930", Name: "삼성전자", Market: "kospi"}})
	yesterday := data.Date(2024, 9, 9)
	if err := store.UpsertIndexBars(ctx, "kospi", []data.IndexBar{{Date: yesterday, Open: 1, High: 1, Low: 1, Close: 1}}); err != nil {
		t.Fatal(err)
	}
	today := data.Date(2024, 9, 10)

	src := &fakeBars{noIndexBars: true} // 지수에 새 봉 없음 (휴장일)
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today, SkipSymbolsIfIndexUnchanged: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped || res.Symbols != 0 {
		t.Errorf("expected Skipped, got %+v", res)
	}
	for _, c := range src.calls {
		if c.kind == "bar" {
			t.Errorf("symbol should not be called: %+v", src.calls)
		}
	}

	// 옵션이 꺼져 있으면 지수가 비어도 종목을 호출한다
	src2 := &fakeBars{noIndexBars: true}
	res, err = RunFetch(ctx, fetchConfig(), store, src2, FetchOptions{Today: today}, nil)
	if err != nil || res.Skipped || res.Symbols != 1 {
		t.Errorf("without option: %+v %v", res, err)
	}

	// 지수에 새 봉이 있으면 옵션이 켜져 있어도 종목을 호출한다
	src3 := &fakeBars{}
	res, err = RunFetch(ctx, fetchConfig(), store, src3, FetchOptions{Today: today, SkipSymbolsIfIndexUnchanged: true}, nil)
	if err != nil || res.Skipped || res.Symbols != 1 {
		t.Errorf("with new index bars: %+v %v", res, err)
	}
}
```

- [ ] **Step 6: RunFetch 수정**

`internal/app/fetch.go`:

```go
type FetchOptions struct {
	From  time.Time // 저장된 봉이 없을 때의 시작일. 비어 있으면 fetch.start_date
	Today time.Time // 비어 있으면 KST 오늘
	// SkipSymbolsIfIndexUnchanged 가 켜져 있고 지수에 새 봉이 하나도 없으면 (주말·공휴일) 종목 호출을 건너뛴다.
	SkipSymbolsIfIndexUnchanged bool
}

type FetchResult struct {
	Symbols  int // 성공한 종목 수
	Bars     int // 저장한 종목 봉 수
	Failures []FetchFailure
	Skipped  bool // 지수 무변화로 종목 수집을 건너뜀
}
```

지수 루프에서 `bars` 저장 직후 `indexNew += len(bars)` 를 세고, 루프 뒤 `syms, err := store.ListSymbols` 앞에:

```go
	if opts.SkipSymbolsIfIndexUnchanged && indexNew == 0 {
		return FetchResult{Skipped: true}, nil
	}
```

Run: `gofmt -l . ; go vet ./... && go test ./internal/config/ ./internal/app/ -count=1 -race 2>&1 | tail -3` → ok.

- [ ] **Step 7: 커밋 요청**

```bash
git add config.yaml internal/config internal/app
git commit -m "feat(app): 자동 수집 스케줄 판정, 지수 무변화 시 종목 건너뛰기"
```

---

### Task 2: TUI 배선 — 자동 수집 루프와 하단 진행 표시

**Files:**
- Modify: `internal/tui/types.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/run.go`, `internal/tui/model_test.go`, `internal/tui/view_test.go`
- Create: `internal/tui/autofetch.go`, `internal/tui/autofetch_test.go`

**Interfaces:**
- Consumes: `app.NextRun`, `app.Stale`, `app.RunFetch`, `app.FetchOptions`, `app.FetchProgress`, `app.FetchResult`, `kis.New`, `data.Open`, `config.KIS.*`, `config.Fetch.DailyAt`, `logfile` 로거.
- Produces: `tui.FetchStatusMsg{Running bool; Done, Total int}`, 비공개 `fetchLoop(ctx, clock func() time.Time, stale func() (bool, error), next func(time.Time) (time.Time, error), run func(context.Context))` (테스트용 주입), `runFetchOnce(ctx, cfg, store, client, logger, send)`. 7단계는 수집 완료 시 관심종목을 다시 계산하려고 `runFetchOnce` 끝에 훅을 단다 (`onDone func()` 인자 추가는 7단계에서).

- [ ] **Step 1: 실패하는 테스트 추가**

`internal/tui/model_test.go` 끝에:

```go
func TestFetchStatusInFooter(t *testing.T) {
	m := sized(t)
	m = send(m, FetchStatusMsg{Running: true, Done: 123, Total: 2500})
	if !strings.Contains(m.View(), "수집 중 123/2500") {
		t.Errorf("footer should show fetch progress:\n%s", m.View())
	}
	m = send(m, FetchStatusMsg{Running: false})
	if strings.Contains(m.View(), "수집 중") {
		t.Errorf("footer should drop progress when done:\n%s", m.View())
	}
}
```

`internal/tui/autofetch_test.go`:

```go
package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchLoopRunsImmediatelyWhenStale(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	done := make(chan struct{})
	go func() {
		fetchLoop(ctx,
			func() time.Time { return base },
			func() (bool, error) { return true, nil },
			func(now time.Time) (time.Time, error) { return now.Add(time.Hour), nil },
			func(context.Context) { runs.Add(1) })
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("stale data should trigger an immediate run")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fetchLoop did not stop on cancel")
	}
}

func TestFetchLoopWaitsForNextRun(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fetchLoop(ctx,
		time.Now,
		func() (bool, error) { return false, nil },
		func(now time.Time) (time.Time, error) { return now.Add(60 * time.Millisecond), nil },
		func(context.Context) { runs.Add(1) })
	time.Sleep(30 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatal("should not run before next time")
	}
	deadline := time.After(2 * time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("did not run at next time")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestFetchLoopStopsOnBadSchedule(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		fetchLoop(ctx, time.Now,
			func() (bool, error) { return false, nil },
			func(time.Time) (time.Time, error) { return time.Time{}, context.DeadlineExceeded },
			func(context.Context) { t.Error("must not run") })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fetchLoop should return when next() errors")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: FetchStatusMsg`, `fetchLoop`)

- [ ] **Step 3: types.go / model.go / view.go**

`types.go` 의 `MarketStatusMsg` 앞에:

```go
// FetchStatusMsg 는 자동 일봉 수집의 진행 상태. Running 이 false 면 하단 표시를 지운다.
type FetchStatusMsg struct {
	Running     bool
	Done, Total int
}
```

`model.go` `Model` 에 필드 추가: `fetch FetchStatusMsg`. `Update` 에 케이스: `case FetchStatusMsg: m.fetch = msg`.

`view.go` 의 `View()` 에서 footer 를 만드는 줄을:

```go
	right := keyHint + " "
	if m.fetch.Running {
		right = fmt.Sprintf("수집 중 %d/%d  ", m.fetch.Done, m.fetch.Total) + right
	}
	footer := spread(m.footerLeft(), right, w)
```

- [ ] **Step 4: autofetch.go**

`internal/tui/autofetch.go`:

```go
package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/app"
	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
)

// fetchLoop 은 데이터가 오래됐으면 즉시 run 을 한 번 부르고, 이후 next 가 주는 시각마다 run 을 부른다. ctx 가 끝나면 반환.
// clock/stale/next/run 은 테스트에서 바꿔 끼운다.
func fetchLoop(ctx context.Context, clock func() time.Time, stale func() (bool, error), next func(time.Time) (time.Time, error), run func(context.Context)) {
	if s, err := stale(); err == nil && s {
		run(ctx)
	}
	for {
		at, err := next(clock())
		if err != nil {
			return
		}
		t := time.NewTimer(time.Until(at))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		if ctx.Err() != nil {
			return
		}
		run(ctx)
	}
}

// runFetchOnce 는 app.RunFetch 를 한 번 돌리며 진행을 화면과 로그에 보낸다.
func runFetchOnce(ctx context.Context, cfg *config.Config, store data.Store, client *kis.Client, logger *slog.Logger, send func(tea.Msg)) {
	logger.Info("자동 수집 시작", "env", cfg.KIS.Env)
	send(FetchStatusMsg{Running: true})
	started := time.Now()
	res, err := app.RunFetch(ctx, cfg, store, client, app.FetchOptions{SkipSymbolsIfIndexUnchanged: true}, func(p app.FetchProgress) {
		if p.Err != nil {
			logger.Warn("종목 수집 실패", "code", p.Code, "err", p.Err)
		}
		send(FetchStatusMsg{Running: true, Done: p.Done, Total: p.Total})
	})
	send(FetchStatusMsg{Running: false})
	switch {
	case err != nil && ctx.Err() != nil:
		logger.Warn("자동 수집 중단 (종료)", "symbols", res.Symbols, "bars", res.Bars)
	case err != nil:
		logger.Error("자동 수집 중단", "err", err)
	case res.Skipped:
		logger.Info("자동 수집 건너뜀 (지수 새 봉 없음 — 휴장일)")
	default:
		logger.Info("자동 수집 완료", "symbols", res.Symbols, "bars", res.Bars, "failed", len(res.Failures), "elapsed", time.Since(started).Round(time.Second).String())
	}
}
```

- [ ] **Step 5: run.go 배선**

`internal/tui/run.go` 의 LS 블록 뒤, `fakeMessages` 고루틴 앞에 추가 (import 에 `app`, `data`, `kis` 추가):

```go
	// 자동 일봉 수집: 앱키·DB 가 있으면 켤 때 따라잡고, 매일 daily_at 에 돈다.
	fetchLog := logger.With("kind", "수집")
	if err := cfg.RequireAppKey(); err != nil {
		fetchLog.Warn("자동 수집 비활성: " + err.Error())
	} else if store, err := data.Open(cfg.DBPath); err != nil {
		fetchLog.Error("자동 수집 비활성: DB 열기 실패", "err", err)
	} else {
		defer store.Close()
		client := kis.New(cfg.KIS.BaseURL(), cfg.KIS.AppKey, cfg.KIS.AppSecret, cfg.KIS.TokenCache, cfg.KIS.EffectiveRPS())
		stale := func() (bool, error) {
			last, has, err := store.LastIndexBarDate(ctx, "kospi")
			if err != nil {
				return false, err
			}
			return app.Stale(last, has, time.Now()), nil
		}
		next := func(now time.Time) (time.Time, error) { return app.NextRun(now, cfg.Fetch.DailyAt) }
		go fetchLoop(ctx, time.Now, stale, next, func(c context.Context) {
			runFetchOnce(c, cfg, store, client, fetchLog, p.Send)
		})
	}
```

`store.Close()` 는 `Run` 끝의 `cancel()` 뒤에 닫혀야 한다. 현재 `Run` 끝은 `cancel(); if closer != nil { closer.Close() }` 순서다 — `defer store.Close()` 는 함수 반환 시 실행되므로 그보다 뒤이고 문제없다.

`Run` 의 doc 주석을 "로그는 …" 뒤에 "한투 앱키가 있으면 일봉을 자동 수집한다." 로 보강.

- [ ] **Step 6: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... -count=1 -race 2>&1 | tail -8`
Expected: 전부 ok. tui 새 테스트 4개.

- [ ] **Step 7: 커밋 요청**

```bash
git add internal/tui
git commit -m "feat(tui): 자동 일봉 수집 — 새벽 daily_at, 켤 때 따라잡기, 하단 진행 표시"
```

---

### Task 3: 스펙 갱신과 수동 확인

- [ ] **Step 1: 스펙 갱신** (`docs/superpowers/specs/2026-09-13-backtest-design.md`)

4절: "TUI는 보기 전용이라 수집을 실행하지 않는다 (수집은 새벽에 `fetch`를 스케줄러로 돌린다)" → "TUI 가 켜져 있으면 매일 `fetch.daily_at`에 전날 일봉을 자동 수집하고, 켤 때 데이터가 오래됐으면 즉시 한 번 받는다. `fetch` 서브커맨드는 수동·스크립트용".
5절 config `fetch:` 에 `daily_at: "04:00"` 줄 추가.
10.1절 하단 행 갱신 열에 "수집 중이면 키 안내 앞에 `수집 중 n/N`" 추가.
11절에 "TUI 자동 수집: 앱키·DB 없으면 비활성(로그 한 줄). 지수 새 봉이 없으면(휴장) 종목 호출 생략. 종료 시 수집 중이던 종목 이후는 다음 실행에서 이어받음" 추가.
13절 나중 단계에서 "실전 자동 실행" 항목의 스케줄러 언급은 그대로 둔다.

- [ ] **Step 2: 수동 확인 (사용자)**

`go run ./cmd/trader` 실행 후 로그 패널:
- 앱키 있고 DB 있으면 `[수집] 자동 수집 시작 env=demo` → 데이터가 최신이면 지수 2건만 확인하고 `자동 수집 건너뜀 (지수 새 봉 없음 — 휴장일)` 또는 (평일 새 봉 있으면) 하단에 `수집 중 n/N` 이 올라가며 `자동 수집 완료 …`.
- 앱키 없으면 `자동 수집 비활성: KIS_APP_KEY …`.
- 다음 날 04:00 이후 TUI 로그에 `자동 수집 시작` 줄이 새로 있어야 한다 (켜둔 경우).

- [ ] **Step 3: 커밋 요청**

```bash
git add docs
git commit -m "docs: TUI 자동 수집 반영"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- TUI 시작 시 데이터가 오래됐으면 수집이 돌고 하단에 진행이 보임; 최신이면 지수 확인만 하고 건너뜀
- 7단계는 `runFetchOnce` 완료 뒤 관심종목 재계산 훅을 단다
