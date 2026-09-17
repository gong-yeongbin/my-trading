package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/app"
	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

// fetchLoop 은 데이터가 오래됐으면 즉시 run 을 한 번 부르고, 이후 next 가 주는 시각마다 run 을 부른다. ctx 가 끝나면 반환.
// clock/stale/next/run 은 테스트에서 바꿔 끼운다.
func fetchLoop(ctx context.Context, clock func() time.Time, stale func() (bool, error), next func(time.Time) (time.Time, error), run func(context.Context)) {
	if s, err := stale(); err == nil && s {
		run(ctx)
	}
	for {
		at, err := next(clock())
		if err != nil {
			return
		}
		// macOS 잠자기 동안 단조 시계가 멈춰 긴 타이머가 늦게 울리므로 1분마다 벽시계로 다시 잰다.
		for {
			remaining := at.Sub(clock())
			if remaining <= 0 {
				break
			}
			if remaining > time.Minute {
				remaining = time.Minute
			}
			t := time.NewTimer(remaining)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		run(ctx)
	}
}

// runFetchOnce 는 app.RunFetch 를 한 번 돌리며 진행을 화면과 로그에 보낸다. 취소가 아니면 끝에 onDone 을 부른다.
func runFetchOnce(ctx context.Context, cfg *config.Config, store data.Store, src app.BarSource, logger *slog.Logger, send func(tea.Msg), onDone func()) {
	logger.Info("자동 수집 시작", "server", "real")
	send(FetchStatusMsg{Running: true})
	started := time.Now()
	// 장중에 켜도 오늘의 미완성 봉을 저장하지 않도록 어제까지만.
	now := time.Now().In(data.KST)
	yesterday := data.Date(now.Year(), now.Month(), now.Day()).AddDate(0, 0, -1)
	res, err := app.RunFetch(ctx, cfg, store, src, app.FetchOptions{Today: yesterday}, func(p app.FetchProgress) {
		if p.Err != nil {
			logger.Warn("종목 수집 실패", "code", p.Code, "err", p.Err)
		}
		send(FetchStatusMsg{Running: true, Done: p.Done, Total: p.Total})
	})
	send(FetchStatusMsg{Running: false})
	switch {
	case err != nil && ctx.Err() != nil:
		logger.Warn("자동 수집 중단 (종료)", "symbols", res.Symbols, "bars", res.Bars)
	case err != nil:
		logger.Error("자동 수집 중단", "err", err)
	case res.Skipped:
		logger.Info("자동 수집 건너뜀 (모든 종목 최신)")
	default:
		logger.Info("자동 수집 완료", "symbols", res.Symbols, "up_to_date", res.UpToDate, "bars", res.Bars, "failed", len(res.Failures), "elapsed", time.Since(started).Round(time.Second).String())
	}
	if err == nil && onDone != nil {
		onDone()
	}
}
