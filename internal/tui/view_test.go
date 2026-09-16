package tui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestViewHasExactHeightAndWidth(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{AsOf: "09-12", Rows: watchRows(50), Filter: MarketFilter{Kospi: "진입가능", Kosdaq: "차단"}})
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("view has %d lines, want 24", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 100 {
			t.Errorf("line %d width %d != 100: %q", i, w, l)
		}
	}
}

func TestViewDisconnectedDefaults(t *testing.T) {
	v := sized(t).View()
	if strings.Count(v, "미연결") < 3 { // 뉴스, 코스피, 코스닥
		t.Errorf("expected 미연결 for news and both indexes:\n%s", v)
	}
	if !strings.Contains(v, "Tab 패널  q 종료") {
		t.Error("key hint missing")
	}
}

func TestViewHeaderAndFooter(t *testing.T) {
	m := sized(t)
	m = send(m, tickMsg(time.Date(2026, 9, 15, 14, 32, 10, 0, time.Local)))
	m = send(m, NewsMsg{Source: "연합뉴스", Title: "삼성전자, 3분기 파운드리 수주 확대 전망"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	m = send(m, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004})
	v := m.View()
	for _, want := range []string{"[연합뉴스] 삼성전자, 3분기 파운드리 수주 확대 전망", "14:32:10", "코스피 2,712.40 ▲+0.8%", "코스닥 782.15 ▼-0.4%"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewWatchPanel(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{AsOf: "09-12", Filter: MarketFilter{Kospi: "진입가능", Kosdaq: "차단"}, Rows: []WatchRow{
		{Name: "삼성전자", Market: "코스피", PrevClose: 71200, MinClose: 73400, MinChangePct: 0.031, MinVolume: 2140000},
	}})
	v := m.View()
	for _, want := range []string{"관심종목  (09-12 기준, 1개)", "코스피 진입가능 · 코스닥 차단", "종목명", "필요거래량", "삼성전자", "71,200", "73,400", "+3.1%", "2,140,000"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewHoldingsPanel(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyEnter)
	if !strings.Contains(m.View(), "보유종목  미연결") {
		t.Errorf("holdings should show 미연결 before data:\n%s", m.View())
	}
	m = send(m, HoldingsMsg{Connected: true})
	if !strings.Contains(m.View(), "보유 없음") {
		t.Errorf("empty holdings should show 보유 없음:\n%s", m.View())
	}
	m = send(m, HoldingsMsg{Connected: true,
		Summary: HoldingsSummary{Total: 12480000, PnL: 312000, PnLPct: 0.026, Cash: 7520000},
		Rows:    []HoldingRow{{Name: "삼성전자", Market: "코스피", Qty: 58, AvgPrice: 71200, Price: 72900, PnL: 98600, PnLPct: 0.024, HoldDays: 1}, {Name: "에코프로", Market: "코스닥", Qty: 40, AvgPrice: 98500, Price: 96100, PnL: -96000, PnLPct: -0.024}},
	})
	v := m.View()
	for _, want := range []string{"보유종목  (2종목)", "평가금액 12,480,000", "손익 +312,000 (+2.6%)", "현금 7,520,000", "+98,600", "+2.4%", "1일", "-96,000", "-2.4%"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewLogPanel(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyEnter)
	m = send(m, LogMsg{Lines: []LogLine{{Time: time.Date(2026, 9, 15, 6, 12, 40, 0, time.Local), Kind: "수집", Msg: "일봉 2,391종목 완료"}}})
	v := m.View()
	for _, want := range []string{"로그", "최신순", "06:12:40", "[수집]", "일봉 2,391종목 완료"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewMenuCursorFollowsFocus(t *testing.T) {
	m := sized(t)
	if !strings.Contains(m.View(), "> 관심종목") {
		t.Error("menu cursor should be on 관심종목 when menu focused")
	}
	m, _ = press(m, keyTab)
	if strings.Contains(m.View(), "> 관심종목") {
		t.Error("menu cursor should disappear when panel focused")
	}
}

func TestViewTruncatesLongText(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Source: "출처", Title: strings.Repeat("긴제목", 60)})
	for i, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("line %d width %d > 100", i, w)
		}
	}
}

func TestFormatHelpers(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1,000", 2140000: "2,140,000", -96000: "-96,000"}
	for n, want := range cases {
		if got := comma(n); got != want {
			t.Errorf("comma(%d) = %q, want %q", n, got, want)
		}
	}
	if got := commaF(2712.4); got != "2,712.40" {
		t.Errorf("commaF = %q", got)
	}
	if got := commaF(math.NaN()); got != "NaN" {
		t.Errorf("commaF(NaN) = %q", got)
	}
	if got := pct(0.031); got != "+3.1%" {
		t.Errorf("pct = %q", got)
	}
	if got := pct(-0.024); got != "-2.4%" {
		t.Errorf("pct = %q", got)
	}
	if got := fit("삼성전자", 10, false); lipgloss.Width(got) != 10 {
		t.Errorf("fit pad width = %d", lipgloss.Width(got))
	}
	if got := fit("삼성전자", 5, false); lipgloss.Width(got) != 5 {
		t.Errorf("fit truncate width = %d: %q", lipgloss.Width(got), got)
	}
	if got := fit("12", 6, true); got != "    12" {
		t.Errorf("fit right = %q", got)
	}
	if got := spread("L", "R", 10); got != "L"+strings.Repeat(" ", 8)+"R" {
		t.Errorf("spread = %q", got)
	}
}

func TestViewNewsWithoutSource(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Title: "출처 없는 제목"})
	v := m.View()
	if !strings.Contains(v, " 출처 없는 제목") || strings.Contains(v, "[] 출처") {
		t.Errorf("news without source should render title only:\n%s", v)
	}
	m = send(m, NewsMsg{Source: "연합뉴스", Title: "출처 있는 제목"})
	if !strings.Contains(m.View(), "[연합뉴스] 출처 있는 제목") {
		t.Errorf("news with source should render [source] title:\n%s", m.View())
	}
}
