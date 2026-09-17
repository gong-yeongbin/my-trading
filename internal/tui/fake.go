package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeMessages 는 아직 실데이터가 없는 패널의 화면 확인용 가짜 데이터다.
// 8단계(보유종목)에서 실데이터로 바꾸며 삭제한다.
func fakeMessages(now time.Time) []tea.Msg {
	return []tea.Msg{
		HoldingsMsg{Connected: true, Summary: HoldingsSummary{Total: 12480000, PnL: 312000, PnLPct: 0.026, Cash: 7520000}, Rows: []HoldingRow{
			{Name: "삼성전자", Market: "코스피", Qty: 58, AvgPrice: 71200, Price: 72900, PnL: 98600, PnLPct: 0.024, HoldDays: 1},
			{Name: "에코프로", Market: "코스닥", Qty: 40, AvgPrice: 98500, Price: 96100, PnL: -96000, PnLPct: -0.024, HoldDays: 1},
			{Name: "현대차", Market: "코스피", Qty: 16, AvgPrice: 245000, Price: 264300, PnL: 308800, PnLPct: 0.079, HoldDays: 2},
		}},
	}
}
