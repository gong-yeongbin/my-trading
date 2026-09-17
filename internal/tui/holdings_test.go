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
	go func() {
		holdingsLoop(ctx, 5*time.Millisecond, now, fetch, send, slog.New(slog.DiscardHandler))
		close(done)
	}()
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
	go func() {
		holdingsLoop(ctx, 5*time.Millisecond, now, fetch, send, slog.New(slog.DiscardHandler))
		close(done)
	}()
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
