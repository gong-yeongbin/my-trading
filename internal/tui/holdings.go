package tui

import (
	"context"
	"log/slog"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/kis"
)

// holdingsMsgFrom 은 잔고를 화면 메시지로 바꾼다. 평가금액(현재가×수량) 큰 순. 보유일은 매매 로그 전까지 "-"(0).
func holdingsMsgFrom(b kis.Balance, mkt func(code string) string) HoldingsMsg {
	msg := HoldingsMsg{Connected: true, Summary: HoldingsSummary{Total: b.Total, PnL: b.PnL, Cash: b.Cash}}
	if b.Purchase > 0 {
		msg.Summary.PnLPct = float64(b.PnL) / float64(b.Purchase)
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

// holdingsRetry 는 첫 성공 전 재시도 간격.
const holdingsRetry = time.Minute

// preOpen 은 매일 잔고를 한 번 조회하는 장전 시각.
var preOpen = struct{ hour, min int }{8, 59}

// holdingsLoop 은 시작 시 한 번, 매일 장전(08:59, 마지막 조회가 그날 08:59 이전이면 — 잠자기로 지나쳤어도 깨어나면) 한 번,
// refresh 신호가 오면 즉시 잔고를 조회한다. every 마다 시계를 확인한다.
// 실패하면 로그만 남기고 마지막 값을 유지하되, 한 번도 성공하지 못했으면 holdingsRetry 간격으로 다시 시도한다.
func holdingsLoop(ctx context.Context, every time.Duration, now func() time.Time, refresh <-chan struct{}, fetch func(context.Context) (HoldingsMsg, error), send func(tea.Msg), logger *slog.Logger) {
	ok := false
	var lastAt time.Time
	poll := func(at time.Time) {
		lastAt = at
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
		msg.At = at
		send(msg)
	}
	poll(now())
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh:
			poll(now())
			continue
		case <-t.C:
		}
		cur := now()
		pre := time.Date(cur.Year(), cur.Month(), cur.Day(), preOpen.hour, preOpen.min, 0, 0, cur.Location())
		if (!cur.Before(pre) && lastAt.Before(pre)) || (!ok && !cur.Before(lastAt.Add(holdingsRetry))) {
			poll(cur)
		}
	}
}
