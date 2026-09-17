package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/settings"
)

const savedMsg = "저장됨 · 재시작하면 적용됩니다"

// settingsEnter 는 설정 패널에서 Enter: 서버 항목은 토글 후 저장, 나머지는 편집 시작.
func (m Model) settingsEnter() (tea.Model, tea.Cmd) {
	if m.settingsErr != nil {
		return m, nil // 읽기에 실패한 값 위에 저장하지 않는다
	}
	f := settings.Field(m.cursor[panelSettings])
	if f == settings.KISEnv {
		v := m.settings
		if v.KISEnv == "real" {
			v.KISEnv = "demo"
		} else {
			v.KISEnv = "real"
		}
		m.save(v)
		return m, nil
	}
	m.editing = true
	m.settingsMsg = ""
	if settings.IsSecret(f) {
		m.input.SetValue("") // 가려진 옛 값을 지울 필요 없이 새 값만 입력. Esc 면 그대로
	} else {
		m.input.SetValue(m.settings.Get(f))
	}
	if settings.IsSecret(f) {
		m.input.EchoMode = textinput.EchoPassword
		m.input.EchoCharacter = '*'
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.CursorEnd()
	return m, m.input.Focus()
}

// handleEditKey 는 편집 중 키. Enter 저장, Esc 취소, 나머지는 입력창으로.
func (m Model) handleEditKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.editing = false
		m.settingsMsg = ""
		m.input.Blur()
		return m, nil
	case "enter":
		f := settings.Field(m.cursor[panelSettings])
		val := m.input.Value()
		if err := settings.Validate(f, val); err != nil {
			m.settingsMsg = "오류: " + err.Error()
			return m, nil
		}
		v := m.settings
		v.Set(f, val)
		m.editing = false
		m.input.Blur()
		m.save(v)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

// save 는 saver 로 저장하고 결과 문구를 남긴다. 성공하면 화면 값도 갱신.
func (m *Model) save(v settings.Values) {
	if m.saver == nil {
		m.settingsMsg = "저장 실패: 저장기 없음"
		return
	}
	if err := m.saver(v); err != nil {
		m.settingsMsg = "저장 실패: " + err.Error()
		return
	}
	m.settings = v
	m.settingsMsg = savedMsg
}

// settingsPanel 은 설정 목록. 편집 중인 항목의 값 칸에는 입력창이 들어간다.
func (m Model) settingsPanel(w int) (string, []column, [][]string) {
	right := "Enter 편집/토글 · Esc 취소 "
	title := spread(" 설정", right, w)
	if m.settingsErr != nil {
		title = spread(" 설정 읽기 실패: "+m.settingsErr.Error(), right, w)
	} else if m.settingsMsg != "" {
		title = spread(" 설정  "+m.settingsMsg, right, w)
	}
	cols := []column{{"항목", 16, false}, {"값", max(10, w-2-16-1), false}}
	body := make([][]string, settings.FieldCount)
	for f := settings.Field(0); f < settings.FieldCount; f++ {
		val := settings.Display(f, m.settings.Get(f))
		if m.editing && int(f) == m.cursor[panelSettings] {
			val = m.input.View()
		}
		body[f] = []string{settings.Labels[f], val}
	}
	return title, cols, body
}
