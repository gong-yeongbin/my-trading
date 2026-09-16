package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func sized(t *testing.T) Model {
	t.Helper()
	m, _ := New().Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	return m.(Model)
}

func press(m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(k)
	return next.(Model), cmd
}

func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

var (
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyQ     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
)

func watchRows(n int) []WatchRow {
	rows := make([]WatchRow, n)
	for i := range rows {
		rows[i] = WatchRow{Name: "종목", Market: "코스피", PrevClose: 1000, MinClose: 1030, MinChangePct: 0.03, MinVolume: 100}
	}
	return rows
}

func TestInitialState(t *testing.T) {
	m := sized(t)
	if m.focus != focusMenu || m.active != panelWatch || m.menuCursor != 0 {
		t.Errorf("initial state: focus=%v active=%v menu=%d", m.focus, m.active, m.menuCursor)
	}
}

func TestTabTogglesFocus(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyTab)
	if m.focus != focusPanel {
		t.Fatal("tab should move focus to panel")
	}
	m, _ = press(m, keyTab)
	if m.focus != focusMenu {
		t.Fatal("tab should move focus back to menu")
	}
}

func TestMenuNavigationAndEnter(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyUp) // 위로 못 감
	if m.menuCursor != 0 {
		t.Errorf("menu cursor went above 0: %d", m.menuCursor)
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, keyDown)
	}
	if m.menuCursor != len(menuLabels)-1 {
		t.Errorf("menu cursor = %d, want %d", m.menuCursor, len(menuLabels)-1)
	}
	m, _ = press(m, keyEnter)
	if m.active != panelLog {
		t.Errorf("enter should activate log panel, got %v", m.active)
	}
	m, _ = press(m, keyUp)
	m, _ = press(m, keyEnter)
	if m.active != panelHoldings {
		t.Errorf("enter should activate holdings panel, got %v", m.active)
	}
}

func TestPanelCursorStaysInBounds(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{AsOf: "09-12", Rows: watchRows(3)})
	m, _ = press(m, keyTab)
	for i := 0; i < 5; i++ {
		m, _ = press(m, keyDown)
	}
	if m.cursor[panelWatch] != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor[panelWatch])
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, keyUp)
	}
	if m.cursor[panelWatch] != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor[panelWatch])
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := sized(t) // 높이 24 → 본문 18줄 → 표 행 14줄
	m = send(m, WatchMsg{AsOf: "09-12", Rows: watchRows(50)})
	m, _ = press(m, keyTab)
	for i := 0; i < 20; i++ {
		m, _ = press(m, keyDown)
	}
	if m.cursor[panelWatch] != 20 {
		t.Fatalf("cursor = %d", m.cursor[panelWatch])
	}
	if got, want := m.offset[panelWatch], 20-m.visibleRows()+1; got != want {
		t.Errorf("offset = %d, want %d (visibleRows=%d)", got, want, m.visibleRows())
	}
	for i := 0; i < 20; i++ {
		m, _ = press(m, keyUp)
	}
	if m.offset[panelWatch] != 0 {
		t.Errorf("offset after scrolling up = %d", m.offset[panelWatch])
	}
}

func TestNewDataClampsCursor(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{Rows: watchRows(10)})
	m, _ = press(m, keyTab)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyDown)
	m = send(m, WatchMsg{Rows: watchRows(5)})
	if m.cursor[panelWatch] != 2 || m.offset[panelWatch] != 0 {
		t.Errorf("cursor/offset not kept: %d/%d", m.cursor[panelWatch], m.offset[panelWatch])
	}
	m = send(m, WatchMsg{Rows: watchRows(1)})
	if m.cursor[panelWatch] != 0 || m.offset[panelWatch] != 0 {
		t.Errorf("cursor/offset not clamped: %d/%d", m.cursor[panelWatch], m.offset[panelWatch])
	}
}

func TestQuit(t *testing.T) {
	m := sized(t)
	_, cmd := press(m, keyQ)
	if cmd == nil {
		t.Fatal("q should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should produce tea.QuitMsg")
	}
}

func TestTickUpdatesClockAndReschedules(t *testing.T) {
	m := sized(t)
	at := time.Date(2026, 9, 15, 14, 32, 10, 0, time.Local)
	next, cmd := m.Update(tickMsg(at))
	if !next.(Model).now.Equal(at) {
		t.Error("now not updated")
	}
	if cmd == nil {
		t.Error("tick should reschedule")
	}
}

func TestDataMessagesStored(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Source: "연합뉴스", Title: "제목"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	m = send(m, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004})
	m = send(m, HoldingsMsg{Connected: true, Rows: []HoldingRow{{Name: "삼성전자"}}})
	m = send(m, LogMsg{Lines: []LogLine{{Kind: "수집", Msg: "시작"}}})
	m = send(m, IndexMsg{Market: "nope", Value: 1})
	if m.kospi.Value != 2712.4 || m.kosdaq.Value != 782.15 {
		t.Errorf("unknown market changed index: %+v %+v", m.kospi, m.kosdaq)
	}
	if !m.newsOK || m.news.Title != "제목" {
		t.Error("news not stored")
	}
	if !m.kospi.Connected || m.kospi.Value != 2712.4 || !m.kosdaq.Connected || m.kosdaq.ChangePct != -0.004 {
		t.Errorf("index not stored: %+v %+v", m.kospi, m.kosdaq)
	}
	if len(m.holdings.Rows) != 1 || len(m.logs) != 1 {
		t.Error("holdings/logs not stored")
	}
}

func TestConnectedShowsWaitingUntilData(t *testing.T) {
	m := sized(t)
	m = send(m, ConnectedMsg{})
	v := m.View()
	if !strings.Contains(v, "연결됨 · 뉴스 대기") || strings.Count(v, "연결됨 · 장외") != 2 || strings.Contains(v, "미연결") {
		t.Errorf("connected without data should show 대기/장외, not 미연결:\n%s", v)
	}
	m = send(m, NewsMsg{Title: "제목"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	v = m.View()
	if strings.Contains(v, "뉴스 대기") || strings.Count(v, "연결됨 · 장외") != 1 || !strings.Contains(v, "2,712.40") {
		t.Errorf("data should replace waiting text:\n%s", v)
	}
	m = send(m, DisconnectedMsg{})
	if strings.Count(m.View(), "미연결") < 3 || strings.Contains(m.View(), "연결됨") {
		t.Errorf("DisconnectedMsg should show 미연결 everywhere:\n%s", m.View())
	}
}

func TestDisconnectedResetsNewsAndIndex(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Title: "제목"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	m = send(m, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004})
	m = send(m, DisconnectedMsg{})
	if m.newsOK || m.kospi.Connected || m.kosdaq.Connected {
		t.Errorf("DisconnectedMsg should reset: newsOK=%v kospi=%v kosdaq=%v", m.newsOK, m.kospi.Connected, m.kosdaq.Connected)
	}
	if strings.Count(m.View(), "미연결") < 3 {
		t.Errorf("view should show 미연결 for news and both indexes:\n%s", m.View())
	}
}
