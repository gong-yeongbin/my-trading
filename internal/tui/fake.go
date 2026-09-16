package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeMessages 는 아직 실데이터가 없는 패널의 화면 확인용 가짜 데이터다.
// 3단계(로그), 7단계(관심종목), 8단계(보유종목)에서 실데이터로 바꾸며 해당 항목을 지우고, 다 지워지면 이 파일을 삭제한다.
func fakeMessages(now time.Time) []tea.Msg {
	return []tea.Msg{
		WatchMsg{AsOf: "09-12", Filter: MarketFilter{Kospi: "진입가능", Kosdaq: "차단"}, Rows: []WatchRow{
			{Name: "삼성전자", Market: "코스피", PrevClose: 71200, MinClose: 73400, MinChangePct: 0.031, MinVolume: 2140000},
			{Name: "SK하이닉스", Market: "코스피", PrevClose: 182000, MinClose: 188100, MinChangePct: 0.034, MinVolume: 620000},
			{Name: "에코프로", Market: "코스닥", PrevClose: 98500, MinClose: 102300, MinChangePct: 0.039, MinVolume: 910000},
			{Name: "현대차", Market: "코스피", PrevClose: 245000, MinClose: 255100, MinChangePct: 0.041, MinVolume: 380000},
			{Name: "LG에너지솔루션", Market: "코스피", PrevClose: 398000, MinClose: 412500, MinChangePct: 0.036, MinVolume: 150000},
			{Name: "셀트리온", Market: "코스피", PrevClose: 178500, MinClose: 184200, MinChangePct: 0.032, MinVolume: 420000},
			{Name: "알테오젠", Market: "코스닥", PrevClose: 312000, MinClose: 325000, MinChangePct: 0.042, MinVolume: 210000},
		}},
		HoldingsMsg{Connected: true, Summary: HoldingsSummary{Total: 12480000, PnL: 312000, PnLPct: 0.026, Cash: 7520000}, Rows: []HoldingRow{
			{Name: "삼성전자", Market: "코스피", Qty: 58, AvgPrice: 71200, Price: 72900, PnL: 98600, PnLPct: 0.024, HoldDays: 1},
			{Name: "에코프로", Market: "코스닥", Qty: 40, AvgPrice: 98500, Price: 96100, PnL: -96000, PnLPct: -0.024, HoldDays: 1},
			{Name: "현대차", Market: "코스피", Qty: 16, AvgPrice: 245000, Price: 264300, PnL: 308800, PnLPct: 0.079, HoldDays: 2},
		}},
		LogMsg{Lines: []LogLine{
			{Time: now.Add(-1 * time.Minute), Kind: "연결", Msg: "LS 웹소켓 재연결 성공"},
			{Time: now.Add(-2 * time.Minute), Kind: "연결", Msg: "LS 웹소켓 연결 끊김, 5초 후 재시도"},
			{Time: now.Add(-5 * time.Hour), Kind: "지수", Msg: "코스피 20일선 위 · 코스닥 20일선 아래 → 코스닥 진입 차단"},
			{Time: now.Add(-8 * time.Hour), Kind: "수집", Msg: "일봉 2,391종목 완료, 실패 3 (000660 타임아웃)"},
			{Time: now.Add(-8*time.Hour - 3*time.Minute), Kind: "수집", Msg: "유니버스 갱신 2,391종목 (+2 −5)"},
			{Time: now.Add(-8*time.Hour - 4*time.Minute), Kind: "수집", Msg: "시작"},
		}},
	}
}
