package tui

import (
	"context"
	"errors"
	"log/slog"
	"sync"
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
		Total: 12480000, Purchase: 12168000, Cash: 7520000, PnL: 312000,
	}
	mk := map[string]string{"005930": "kospi", "247540": "kosdaq"}
	msg := holdingsMsgFrom(b, func(code string) string { return mk[code] })
	if !msg.Connected || msg.Summary.Total != 12480000 || msg.Summary.Cash != 7520000 || msg.Summary.PnL != 312000 {
		t.Errorf("summary = %+v", msg.Summary)
	}
	if msg.Summary.PnLPct < 0.0256 || msg.Summary.PnLPct > 0.0257 { // 312000 / 12168000
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

// runLoop 은 주입한 시계로 루프를 잠깐 돌리고 fetch 횟수와 보낸 메시지를 돌려준다.
func runLoop(t *testing.T, now func() time.Time, refresh chan struct{}, fetch func(context.Context) (HoldingsMsg, error), during func()) []tea.Msg {
	t.Helper()
	var mu sync.Mutex
	var msgs []tea.Msg
	send := func(m tea.Msg) { mu.Lock(); msgs = append(msgs, m); mu.Unlock() }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		holdingsLoop(ctx, 5*time.Millisecond, now, refresh, fetch, send, slog.New(slog.DiscardHandler))
		close(done)
	}()
	during()
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	return msgs
}

// seqClock 은 호출마다 다음 시각을 돌려주고, 다 쓰면 마지막 값을 유지한다.
func seqClock(ticks ...time.Time) func() time.Time {
	var i atomic.Int32
	return func() time.Time {
		n := int(i.Add(1)) - 1
		if n < len(ticks) {
			return ticks[n]
		}
		return ticks[len(ticks)-1]
	}
}

func TestHoldingsLoopPollsAtStartAndPreOpenOnce(t *testing.T) {
	var fetches atomic.Int32
	d := func(h, m int) time.Time { return time.Date(2026, 9, 17, h, m, 0, 0, market.KST) }
	// 07:00 에 켬 → 08:58 → 08:59(조회) → 09:00 → 10:00 → 15:35(조회 없음)
	now := seqClock(d(7, 0), d(8, 58), d(8, 59), d(9, 0), d(10, 0), d(15, 35))
	fetch := func(context.Context) (HoldingsMsg, error) { fetches.Add(1); return HoldingsMsg{Connected: true}, nil }
	msgs := runLoop(t, now, make(chan struct{}), fetch, func() { time.Sleep(100 * time.Millisecond) })
	if got := fetches.Load(); got != 2 {
		t.Errorf("fetches = %d, want 2 (시작 + 08:59)", got)
	}
	if len(msgs) != 2 || !msgs[1].(HoldingsMsg).At.Equal(d(8, 59)) {
		t.Errorf("msgs = %+v", msgs)
	}
}

func TestHoldingsLoopSkipsPreOpenWhenStartedAfterIt(t *testing.T) {
	var fetches atomic.Int32
	d := func(h, m int) time.Time { return time.Date(2026, 9, 17, h, m, 0, 0, market.KST) }
	now := seqClock(d(10, 0), d(10, 1), d(10, 2), d(11, 0))
	fetch := func(context.Context) (HoldingsMsg, error) { fetches.Add(1); return HoldingsMsg{Connected: true}, nil }
	runLoop(t, now, make(chan struct{}), fetch, func() { time.Sleep(60 * time.Millisecond) })
	if got := fetches.Load(); got != 1 {
		t.Errorf("fetches = %d, want 1 (시작 조회가 오늘 장전 조회를 대신함)", got)
	}
}

func TestHoldingsLoopPollsPreOpenAfterSleepingThroughIt(t *testing.T) {
	var fetches atomic.Int32
	d := func(h, m int) time.Time { return time.Date(2026, 9, 17, h, m, 0, 0, market.KST) }
	// 07:00 에 켜고 잠들었다가 09:30 에 깸 → 그때 1회
	now := seqClock(d(7, 0), d(9, 30), d(9, 31), d(9, 32))
	fetch := func(context.Context) (HoldingsMsg, error) { fetches.Add(1); return HoldingsMsg{Connected: true}, nil }
	runLoop(t, now, make(chan struct{}), fetch, func() { time.Sleep(60 * time.Millisecond) })
	if got := fetches.Load(); got != 2 {
		t.Errorf("fetches = %d, want 2", got)
	}
}

func TestHoldingsLoopRefreshPollsImmediately(t *testing.T) {
	var fetches atomic.Int32
	now := func() time.Time { return time.Date(2026, 9, 17, 20, 0, 0, 0, market.KST) }
	fetch := func(context.Context) (HoldingsMsg, error) { fetches.Add(1); return HoldingsMsg{Connected: true}, nil }
	refresh := make(chan struct{}, 1)
	runLoop(t, now, refresh, fetch, func() {
		time.Sleep(20 * time.Millisecond)
		refresh <- struct{}{}
		time.Sleep(20 * time.Millisecond)
		refresh <- struct{}{}
		time.Sleep(20 * time.Millisecond)
	})
	if got := fetches.Load(); got != 3 {
		t.Errorf("fetches = %d, want 3 (시작 + 새로고침 2)", got)
	}
}

func TestHoldingsLoopKeepsLastOnErrorAndReportsFirstFailure(t *testing.T) {
	// 장외 20:00. 호출마다 시계가 1분씩 흐른다 → 재시도 간격을 매 틱 넘긴다.
	base := time.Date(2026, 9, 17, 20, 0, 0, 0, market.KST)
	var step atomic.Int32
	now := func() time.Time { return base.Add(time.Duration(step.Add(1)) * time.Minute) }
	var calls atomic.Int32
	fetch := func(context.Context) (HoldingsMsg, error) {
		if calls.Add(1) < 3 {
			return HoldingsMsg{}, errors.New("boom")
		}
		return HoldingsMsg{Connected: true, Rows: []HoldingRow{{Name: "삼성전자"}}}, nil
	}
	msgs := runLoop(t, now, make(chan struct{}), fetch, func() { time.Sleep(100 * time.Millisecond) })
	// 실패 2회 뒤 3회째 성공, 이후 장외라 더 조회하지 않는다.
	if got := calls.Load(); got != 3 {
		t.Errorf("fetches = %d, want 3", got)
	}
	// 성공 전 실패는 미연결을 보내고, 성공은 표를 보낸다.
	if len(msgs) != 3 {
		t.Fatalf("msgs = %d, want 3", len(msgs))
	}
	if first := msgs[0].(HoldingsMsg); first.Connected {
		t.Error("failure with no prior data should send 미연결")
	}
	if last := msgs[2].(HoldingsMsg); !last.Connected || len(last.Rows) != 1 {
		t.Errorf("last = %+v", last)
	}
}
