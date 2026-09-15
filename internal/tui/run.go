package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
)

// Run 은 전체 화면 TUI 를 띄우고 종료될 때까지 막는다. cfg 는 2단계(LS)·8단계(잔고)부터 쓴다.
func Run(ctx context.Context, cfg *config.Config) error {
	_ = cfg
	// 취소 가능한 자식 컨텍스트: 이후 단계에서 Run 안에서 시작하는 고루틴들이 q 로 TUI 종료 시 함께 멈추도록.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithContext(ctx))
	go func() {
		for _, msg := range fakeMessages(time.Now()) {
			p.Send(msg)
		}
	}()
	_, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil // Ctrl+C 로 컨텍스트가 취소된 정상 종료
	}
	return err
}
