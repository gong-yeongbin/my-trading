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

// holdingsLoop 은 시작 시 한 번, 이후 every 마다 거래 시간대(market.At(now).Trading())면 잔고를 조회한다.
// 거래 시간대가 끝난 직후 한 번 더 조회해 종가를 반영한다. 실패하면 로그만 남기고 마지막 값을 유지한다.
// 한 번도 성공하지 못했으면 장외에도 holdingsRetry 간격으로 다시 시도한다.
// holdingsRetry 는 첫 성공 전 장외 재시도 간격.
const holdingsRetry = time.Minute

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
	cur := now()
	wasTrading := market.At(cur).Trading()
	retryAt := cur.Add(holdingsRetry)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur = now()
		trading := market.At(cur).Trading()
		if trading || wasTrading || (!ok && !cur.Before(retryAt)) {
			poll()
			retryAt = cur.Add(holdingsRetry)
		}
		wasTrading = trading
	}
}
