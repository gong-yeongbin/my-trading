package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/market"
)

type focus int

const (
	focusMenu focus = iota
	focusPanel
)

type panel int

const (
	panelWatch panel = iota
	panelHoldings
	panelLog
	panelCount
)

var menuLabels = [panelCount]string{"관심종목", "보유종목", "로그"}

// 고정 줄 수: 상단 괘선, 뉴스, 괘선, (본문), 괘선, 하단 정보, 괘선
const chromeLines = 6

// 패널 안에서 표 위에 쓰는 줄 수: 제목, 빈 줄, 헤더, 괘선
const panelHeaderLines = 4

type indexQuote struct {
	Value     float64
	ChangePct float64
	Connected bool
}

type tickMsg time.Time

type Model struct {
	width, height int
	now           time.Time

	focus      focus
	menuCursor int
	active     panel
	cursor     [panelCount]int
	offset     [panelCount]int

	linkOK    bool      // LS 연결·구독 완료 여부
	jifStatus string    // JIF 로 받은 장 상태. 비어 있으면 시계 기준
	jifAt     time.Time // jifStatus 를 받은 시각
	news      NewsMsg
	newsOK    bool
	kospi     indexQuote
	kosdaq    indexQuote
	watch     WatchMsg
	holdings  HoldingsMsg
	logs      []LogLine
	fetch     FetchStatusMsg
}

func New() Model {
	return Model{now: time.Now()}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd { return tick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case NewsMsg:
		m.news, m.newsOK = msg, true
	case IndexMsg:
		q := indexQuote{Value: msg.Value, ChangePct: msg.ChangePct, Connected: true}
		switch msg.Market {
		case "kospi":
			m.kospi = q
		case "kosdaq":
			m.kosdaq = q
		}
	case WatchMsg:
		m.watch = msg
		m.cursor[panelWatch] = min(m.cursor[panelWatch], max(0, m.rowCount(panelWatch)-1))
		if panelWatch == m.active {
			m.clampScroll()
		}
	case HoldingsMsg:
		m.holdings = msg
		m.cursor[panelHoldings] = min(m.cursor[panelHoldings], max(0, m.rowCount(panelHoldings)-1))
		if panelHoldings == m.active {
			m.clampScroll()
		}
	case LogMsg:
		m.logs = msg.Lines
		m.cursor[panelLog] = min(m.cursor[panelLog], max(0, m.rowCount(panelLog)-1))
		if panelLog == m.active {
			m.clampScroll()
		}
	case FetchStatusMsg:
		m.fetch = msg
	case MarketStatusMsg:
		m.jifStatus, m.jifAt = msg.Status, m.now
	case ConnectedMsg:
		m.linkOK = true
	case DisconnectedMsg:
		m.linkOK = false
		m.newsOK = false
		m.kospi.Connected = false
		m.kosdaq.Connected = false
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		if m.focus == focusMenu {
			m.focus = focusPanel
		} else {
			m.focus = focusMenu
		}
	case "up":
		if m.focus == focusMenu {
			m.menuCursor = max(0, m.menuCursor-1)
		} else {
			m.cursor[m.active] = max(0, m.cursor[m.active]-1)
		}
	case "down":
		if m.focus == focusMenu {
			m.menuCursor = min(int(panelCount)-1, m.menuCursor+1)
		} else {
			m.cursor[m.active] = min(max(0, m.rowCount(m.active)-1), m.cursor[m.active]+1)
		}
	case "enter":
		if m.focus == focusMenu {
			m.active = panel(m.menuCursor)
		}
	}
	m.clampScroll()
	return m, nil
}

func (m Model) rowCount(p panel) int {
	switch p {
	case panelWatch:
		return len(m.watch.Rows)
	case panelHoldings:
		return len(m.holdings.Rows)
	default:
		return len(m.logs)
	}
}

// visibleRows 는 표 본문에 그릴 수 있는 행 수. 터미널이 작아도 최소 1.
func (m Model) visibleRows() int {
	return max(1, m.height-chromeLines-panelHeaderLines)
}

// clampScroll 은 활성 패널의 커서가 화면 안에 오도록 offset 을 조정한다.
func (m *Model) clampScroll() {
	p := m.active
	vis := m.visibleRows()
	if m.cursor[p] < m.offset[p] {
		m.offset[p] = m.cursor[p]
	}
	if m.cursor[p] >= m.offset[p]+vis {
		m.offset[p] = m.cursor[p] - vis + 1
	}
	if m.offset[p] < 0 {
		m.offset[p] = 0
	}
}

// marketStatus 는 하단 줄에 붙일 장 상태. 같은 날 JIF 를 받았으면 그 값, 아니면 시계 기준.
func (m Model) marketStatus() string {
	if m.jifStatus != "" {
		a, b := m.jifAt.In(market.KST), m.now.In(market.KST)
		if a.Year() == b.Year() && a.YearDay() == b.YearDay() {
			return m.jifStatus
		}
	}
	return market.Status(m.now)
}

// View 는 view.go 에 있다.
