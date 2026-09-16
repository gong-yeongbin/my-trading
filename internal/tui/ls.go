package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/ls"
)

// lsSubscriptions 는 TUI 가 구독하는 LS 실시간 TR. 업종코드 001 코스피 종합, 301 코스닥 종합.
var lsSubscriptions = []ls.Subscription{{TrCd: "NWS", TrKey: "NWS001"}, {TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"}}

var lsMarkets = map[string]string{"001": "kospi", "301": "kosdaq"}

// lsToMsg 는 ls 이벤트를 화면 메시지로 바꾼다. 표시할 게 없으면 nil.
func lsToMsg(ev ls.Event) tea.Msg {
	switch e := ev.(type) {
	case ls.News:
		return NewsMsg{Title: e.Title}
	case ls.Index:
		market, ok := lsMarkets[e.Code]
		if !ok {
			return nil
		}
		return IndexMsg{Market: market, Value: e.Value, ChangePct: e.ChangePct}
	case ls.Connected:
		return ConnectedMsg{}
	case ls.Disconnected:
		return DisconnectedMsg{}
	}
	return nil
}
