package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/settings"
)

var keyEsc = tea.KeyMsg{Type: tea.KeyEsc}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// openSettings 는 메뉴에서 설정을 고르고 패널로 포커스를 옮긴다.
func openSettings(t *testing.T, saved *settings.Values, saveErr error) Model {
	t.Helper()
	m := sized(t)
	m.saver = func(v settings.Values) error {
		if saveErr != nil {
			return saveErr
		}
		*saved = v
		return nil
	}
	m = send(m, SettingsMsg{Values: settings.Values{KISEnv: "demo", KISDemoKey: "PSdemo1234", DailyAt: "04:00", StartDate: "2025-09-01"}})
	for i := 0; i < int(panelSettings); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter) // 선택과 함께 패널로 포커스 이동
	return m
}

func TestSettingsPanelShowsMaskedValues(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	v := m.View()
	for _, want := range []string{"설정", "매매 서버", "demo", "한투 모의 앱키", "PSde****", "자동 수집 시각", "04:00", "(없음)"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "PSdemo1234") {
		t.Error("secret must be masked")
	}
}

func TestSettingsToggleEnvSavesImmediately(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	m, _ = press(m, keyEnter) // 커서 0 = 매매 서버
	if saved.KISEnv != "real" || m.settings.KISEnv != "real" {
		t.Errorf("toggle should save real: saved=%+v", saved)
	}
	if !strings.Contains(m.View(), "저장됨 · 재시작하면 적용됩니다") {
		t.Errorf("save message missing:\n%s", m.View())
	}
	m, _ = press(m, keyEnter)
	if saved.KISEnv != "demo" {
		t.Errorf("toggle back: %+v", saved)
	}
}

func TestSettingsEditSaveAndCancel(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	// 자동 수집 시각으로 이동
	for i := 0; i < int(settings.DailyAt); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	if !m.editing {
		t.Fatal("enter should start editing")
	}
	// 편집 중엔 q 가 글자
	m = typeText(m, "q")
	if m.input.Value() != "04:00q" {
		t.Errorf("q should be typed, got %q", m.input.Value())
	}
	m, _ = press(m, keyEsc)
	if m.editing || m.settings.DailyAt != "04:00" {
		t.Errorf("esc should cancel: editing=%v value=%q", m.editing, m.settings.DailyAt)
	}
	// 다시 편집해 저장
	m, _ = press(m, keyEnter)
	m.input.SetValue("")
	m = typeText(m, "05:30")
	m, _ = press(m, keyEnter)
	if m.editing || m.settings.DailyAt != "05:30" || saved.DailyAt != "05:30" {
		t.Errorf("enter should save: editing=%v value=%q saved=%+v", m.editing, m.settings.DailyAt, saved)
	}
	if !strings.Contains(m.View(), "저장됨") {
		t.Errorf("save message missing:\n%s", m.View())
	}
}

func TestSettingsRejectsInvalidAndStaysEditing(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	for i := 0; i < int(settings.DailyAt); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	m.input.SetValue("")
	m = typeText(m, "5:30")
	m, _ = press(m, keyEnter)
	if !m.editing || saved.DailyAt != "" {
		t.Errorf("invalid value must not save: editing=%v saved=%+v", m.editing, saved)
	}
	if !strings.Contains(m.View(), "HH:MM") {
		t.Errorf("validation message missing:\n%s", m.View())
	}
}

func TestSettingsSecretInputIsMasked(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	m, _ = press(m, keyDown) // 한투 모의 앱키
	m, _ = press(m, keyEnter)
	if m.input.Value() != "" {
		t.Errorf("secret edit should start empty, got %q", m.input.Value())
	}
	m = typeText(m, "PSnew")
	if strings.Contains(m.View(), "PSnew") {
		t.Errorf("secret input must be masked while typing:\n%s", m.View())
	}
	m, _ = press(m, keyEnter)
	if saved.KISDemoKey != "PSnew" {
		t.Errorf("saved = %+v", saved)
	}
}

func TestSettingsSaveErrorShown(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, errors.New("disk full"))
	m, _ = press(m, keyEnter) // 서버 토글 → 저장 실패
	if !strings.Contains(m.View(), "저장 실패: disk full") {
		t.Errorf("error message missing:\n%s", m.View())
	}
}

func TestSettingsLoadErrorShown(t *testing.T) {
	m := sized(t)
	m = send(m, SettingsMsg{Err: errors.New("no yaml")})
	for i := 0; i < int(panelSettings); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	if !strings.Contains(m.View(), "설정 읽기 실패: no yaml") {
		t.Errorf("load error missing:\n%s", m.View())
	}
}

func TestSettingsEscClearsMessageAndErrBlocksEdit(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	for i := 0; i < int(settings.DailyAt); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	m.input.SetValue("")
	m = typeText(m, "bad")
	m, _ = press(m, keyEnter) // 검증 실패 → 오류 문구
	m, _ = press(m, keyEsc)
	if strings.Contains(m.View(), "오류:") {
		t.Errorf("esc should clear the validation message:\n%s", m.View())
	}
	m2 := sized(t)
	m2.saver = func(settings.Values) error { t.Error("must not save"); return nil }
	m2 = send(m2, SettingsMsg{Err: errors.New("no yaml")})
	for i := 0; i < int(panelSettings); i++ {
		m2, _ = press(m2, keyDown)
	}
	m2, _ = press(m2, keyEnter)
	m2, _ = press(m2, keyEnter) // 서버 토글 시도 → 막혀야 함
	if m2.editing {
		t.Error("editing must not start when settings failed to load")
	}
}
