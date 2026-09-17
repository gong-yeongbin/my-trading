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
		if ctx.Err() != nil {
			return
		}
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
