package tui

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

func TestFetchLoopRunsImmediatelyWhenStale(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	done := make(chan struct{})
	go func() {
		fetchLoop(ctx,
			func() time.Time { return base },
			func() (bool, error) { return true, nil },
			func(now time.Time) (time.Time, error) { return now.Add(time.Hour), nil },
			func(context.Context) { runs.Add(1) })
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("stale data should trigger an immediate run")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fetchLoop did not stop on cancel")
	}
}

func TestFetchLoopWaitsForNextRun(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fetchLoop(ctx,
		time.Now,
		func() (bool, error) { return false, nil },
		func(now time.Time) (time.Time, error) { return now.Add(60 * time.Millisecond), nil },
		func(context.Context) { runs.Add(1) })
	time.Sleep(30 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatal("should not run before next time")
	}
	deadline := time.After(2 * time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("did not run at next time")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

type fakeBarSource struct {
	indexTo []time.Time
	barTo   []time.Time
}

func (f *fakeBarSource) DailyBars(_ context.Context, code string, from, to time.Time) ([]data.Bar, error) {
	f.barTo = append(f.barTo, to)
	return []data.Bar{{Date: from, Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}}, nil
}

func (f *fakeBarSource) IndexBars(_ context.Context, market string, from, to time.Time) ([]data.IndexBar, error) {
	f.indexTo = append(f.indexTo, to)
	return []data.IndexBar{{Date: from, Open: 1, High: 1, Low: 1, Close: 1}}, nil
}

func TestRunFetchOnceFetchesThroughYesterday(t *testing.T) {
	ctx := context.Background()
	store, err := data.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertSymbols(ctx, []data.Symbol{{Code: "005930", Name: "삼성전자", Market: "kospi"}}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Universe: config.UniverseConfig{Markets: []string{"kospi"}}, Fetch: config.FetchConfig{StartDate: "2024-01-01"}}
	logger := slog.New(slog.DiscardHandler)
	src := &fakeBarSource{}
	var msgs []tea.Msg
	send := func(m tea.Msg) { msgs = append(msgs, m) }

	runFetchOnce(ctx, cfg, store, src, logger, send)

	now := time.Now().In(data.KST)
	yesterday := data.Date(now.Year(), now.Month(), now.Day()).AddDate(0, 0, -1)
	for _, to := range src.indexTo {
		if !to.Equal(yesterday) {
			t.Errorf("index fetched to %v, want %v", to, yesterday)
		}
	}
	for _, to := range src.barTo {
		if !to.Equal(yesterday) {
			t.Errorf("symbol fetched to %v, want %v", to, yesterday)
		}
	}
	if len(msgs) == 0 {
		t.Fatal("expected messages")
	}
	last, ok := msgs[len(msgs)-1].(FetchStatusMsg)
	if !ok || last.Running {
		t.Errorf("last message = %+v, want FetchStatusMsg{Running:false}", msgs[len(msgs)-1])
	}
}

func TestFetchLoopStopsOnBadSchedule(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		fetchLoop(ctx, time.Now,
			func() (bool, error) { return false, nil },
			func(time.Time) (time.Time, error) { return time.Time{}, context.DeadlineExceeded },
			func(context.Context) { t.Error("must not run") })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fetchLoop should return when next() errors")
	}
}
