package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const menuWidth = 14 // "  > 관심종목  "

// 색·굵기 스타일은 이 단계에서 쓰지 않는다. ANSI 코드가 끼면 문구 포함 테스트가 깨지고, 폭 계산도 복잡해진다.
const keyHint = "Tab 패널  q 종료"

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	w := m.width
	rule := strings.Repeat("─", w)
	header := spread(m.headerLeft(), m.now.Format("15:04:05")+" ", w)
	footer := spread(m.footerLeft(), keyHint+" ", w)
	lines := make([]string, 0, m.height)
	lines = append(lines, rule, header, rule)
	lines = append(lines, m.bodyLines()...)
	lines = append(lines, rule, footer, rule)
	return strings.Join(lines, "\n")
}

func (m Model) headerLeft() string {
	if !m.newsOK {
		return " 미연결"
	}
	return fmt.Sprintf(" [%s] %s", m.news.Source, m.news.Title)
}

func (m Model) footerLeft() string {
	return " 코스피 " + fmtIndex(m.kospi) + "   코스닥 " + fmtIndex(m.kosdaq)
}

func fmtIndex(q indexQuote) string {
	if !q.Connected {
		return "미연결"
	}
	arrow := "▲"
	if q.ChangePct < 0 {
		arrow = "▼"
	}
	return commaF(q.Value) + " " + arrow + pct(q.ChangePct)
}

// bodyLines 는 왼쪽 메뉴와 가운데 패널을 한 줄씩 붙여 본문 줄들을 만든다.
func (m Model) bodyLines() []string {
	rows := max(1, m.height-chromeLines)
	panelW := m.width - menuWidth - 1
	menu := m.menuLines(rows)
	panel := m.panelLines(panelW, rows)
	out := make([]string, rows)
	for i := range out {
		out[i] = fit(menu[i], menuWidth, false) + "│" + fit(panel[i], panelW, false)
	}
	return out
}

func (m Model) menuLines(rows int) []string {
	out := make([]string, rows)
	for i, label := range menuLabels {
		if i+1 >= rows {
			break
		}
		marker := "  "
		if m.focus == focusMenu && m.menuCursor == i {
			marker = "> "
		}
		out[i+1] = "  " + marker + label
	}
	return out
}

func (m Model) panelLines(w, rows int) []string {
	var title string
	var cols []column
	var body [][]string
	switch m.active {
	case panelWatch:
		title, cols, body = m.watchPanel(w)
	case panelHoldings:
		title, cols, body = m.holdingsPanel(w)
	default:
		title, cols, body = m.logPanel(w)
	}
	out := []string{title, ""}
	out = append(out, renderTable(cols, body, m.cursor[m.active], m.offset[m.active], m.visibleRows(), m.focus == focusPanel)...)
	for len(out) < rows {
		out = append(out, "")
	}
	return out[:rows]
}

type column struct {
	title string
	width int
	right bool
}

// renderTable 은 헤더, 괘선, 보이는 행들을 돌려준다. body 가 nil 이면 rows 자리에 빈 줄만 남긴다.
func renderTable(cols []column, body [][]string, cursor, offset, visible int, focused bool) []string {
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = fit(c.title, c.width, c.right)
	}
	total := 2 // 커서 표시 폭
	for _, c := range cols {
		total += c.width + 1
	}
	out := []string{"  " + strings.Join(head, " "), strings.Repeat("─", total)}
	end := min(len(body), offset+visible)
	for i := offset; i < end; i++ {
		marker := "  "
		if focused && i == cursor {
			marker = "> "
		}
		cells := make([]string, len(cols))
		for j, c := range cols {
			cells[j] = fit(body[i][j], c.width, c.right)
		}
		out = append(out, marker+strings.Join(cells, " "))
	}
	return out
}

func (m Model) watchPanel(w int) (string, []column, [][]string) {
	f := m.watch.Filter
	if f.Kospi == "" {
		f.Kospi, f.Kosdaq = "알 수 없음", "알 수 없음"
	}
	asOf := m.watch.AsOf
	if asOf == "" {
		asOf = "-"
	}
	title := spread(fmt.Sprintf(" 관심종목  (%s 기준, %d개)", asOf, len(m.watch.Rows)),
		fmt.Sprintf("코스피 %s · 코스닥 %s ", f.Kospi, f.Kosdaq), w)
	cols := []column{{"종목명", 16, false}, {"시장", 6, false}, {"전일종가", 10, true}, {"필요종가", 10, true}, {"필요상승", 8, true}, {"필요거래량", 12, true}}
	body := make([][]string, len(m.watch.Rows))
	for i, r := range m.watch.Rows {
		body[i] = []string{r.Name, r.Market, comma(r.PrevClose), comma(r.MinClose), pct(r.MinChangePct), comma(r.MinVolume)}
	}
	return title, cols, body
}

func (m Model) holdingsPanel(w int) (string, []column, [][]string) {
	cols := []column{{"종목명", 16, false}, {"시장", 6, false}, {"수량", 6, true}, {"매입가", 10, true}, {"현재가", 10, true}, {"손익", 12, true}, {"수익률", 8, true}, {"보유일", 6, true}}
	h := m.holdings
	if !h.Connected {
		return " 보유종목  미연결", cols, nil
	}
	if len(h.Rows) == 0 {
		return " 보유종목  보유 없음", cols, nil
	}
	s := h.Summary
	title := spread(fmt.Sprintf(" 보유종목  (%d종목)", len(h.Rows)),
		fmt.Sprintf("평가금액 %s   손익 %s (%s)   현금 %s ", comma(s.Total), signed(s.PnL), pct(s.PnLPct), comma(s.Cash)), w)
	body := make([][]string, len(h.Rows))
	for i, r := range h.Rows {
		days := "-"
		if r.HoldDays > 0 {
			days = strconv.Itoa(r.HoldDays) + "일"
		}
		body[i] = []string{r.Name, r.Market, comma(r.Qty), comma(r.AvgPrice), comma(r.Price), signed(r.PnL), pct(r.PnLPct), days}
	}
	return title, cols, body
}

func (m Model) logPanel(w int) (string, []column, [][]string) {
	msgW := max(10, w-20) // 커서 2 + 시각 8 + 종류 6 + 구분 공백 3, 여유 1
	cols := []column{{"시각", 8, false}, {"종류", 6, false}, {"메시지", msgW, false}}
	body := make([][]string, len(m.logs))
	for i, l := range m.logs {
		body[i] = []string{l.Time.Format("15:04:05"), "[" + l.Kind + "]", l.Msg}
	}
	return spread(" 로그", "최신순 ", w), cols, body
}

// fit 은 s 를 폭 w 에 맞춘다. 길면 "…" 로 자르고 짧으면 공백을 채운다 (right 면 오른쪽 정렬).
func fit(s string, w int, right bool) string {
	s = ansi.Truncate(s, w, "…")
	pad := strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
	if right {
		return pad + s
	}
	return s + pad
}

// spread 는 left 를 왼쪽, right 를 오른쪽 끝에 놓고 사이를 공백으로 채운 폭 w 의 한 줄을 만든다.
func spread(left, right string, w int) string {
	rw := lipgloss.Width(right)
	left = ansi.Truncate(left, max(0, w-rw-1), "…")
	gap := max(1, w-lipgloss.Width(left)-rw)
	return left + strings.Repeat(" ", gap) + right
}

func comma(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func commaF(v float64) string {
	s := fmt.Sprintf("%.2f", v) // "2712.40"
	dot := strings.LastIndex(s, ".")
	if dot < 0 {
		return s
	}
	whole, _ := strconv.ParseInt(s[:dot], 10, 64)
	return comma(whole) + s[dot:]
}

func signed(n int64) string {
	if n > 0 {
		return "+" + comma(n)
	}
	return comma(n)
}

func pct(v float64) string {
	return fmt.Sprintf("%+.1f%%", v*100)
}
