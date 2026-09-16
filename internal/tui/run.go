package tui

import (
	"context"
	"errors"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/ls"
)

// Run 은 전체 화면 TUI 를 띄우고 종료될 때까지 막는다.
// LS 앱키가 있으면 실시간 뉴스·지수를 구독해 화면에 밀어 넣는다. 로그는 3단계에서 파일로 보낸다.
func Run(ctx context.Context, cfg *config.Config) error {
	// 취소 가능한 자식 컨텍스트: Run 안에서 시작하는 고루틴들이 q 로 TUI 종료 시 함께 멈추도록.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithContext(ctx))

	if cfg.LS.HasAppKey() {
		client := ls.New(ls.Config{
			BaseURL: cfg.LS.BaseURL, WSURL: cfg.LS.WSURL,
			AppKey: cfg.LS.AppKey, AppSecret: cfg.LS.AppSecret, TokenCache: cfg.LS.TokenCache,
		}, slog.New(slog.DiscardHandler))
		events := make(chan ls.Event, 64)
		go client.Run(ctx, lsSubscriptions, events)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ev := <-events:
					if msg := lsToMsg(ev); msg != nil {
						p.Send(msg)
					}
				}
			}
		}()
	}

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
