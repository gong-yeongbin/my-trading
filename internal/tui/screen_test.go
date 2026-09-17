package tui

import (
	"testing"

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
}
