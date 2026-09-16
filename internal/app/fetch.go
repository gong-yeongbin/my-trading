package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

type BarSource interface {
	DailyBars(ctx context.Context, code string, from, to time.Time) ([]data.Bar, error)
	IndexBars(ctx context.Context, market string, from, to time.Time) ([]data.IndexBar, error)
}

type FetchOptions struct {
	From  time.Time // 저장된 봉이 없을 때의 시작일. 비어 있으면 fetch.start_date
	Today time.Time // 비어 있으면 KST 오늘
}

type FetchProgress struct {
	Done  int
	Total int
	Code  string
	Err   error
}

type FetchFailure struct {
	Code string
	Err  error
}

type FetchResult struct {
	Symbols  int // 성공한 종목 수
	Bars     int // 저장한 봉 수
	Failures []FetchFailure
}

// RunFetch 는 지수 일봉을 먼저(실패 시 즉시 중단), 이어서 종목 일봉을 증분 수집한다.
// 종목 하나가 실패해도 계속 진행하고 Failures 에 기록한다.
func RunFetch(ctx context.Context, cfg *config.Config, store data.Store, src BarSource, opts FetchOptions, progress func(FetchProgress)) (FetchResult, error) {
	today := opts.Today
	if today.IsZero() {
		now := time.Now().In(data.KST)
		today = data.Date(now.Year(), now.Month(), now.Day())
	}
	start := opts.From
	if start.IsZero() {
		var err error
		if start, err = data.ParseDate(cfg.Fetch.StartDate); err != nil {
			return FetchResult{}, fmt.Errorf("fetch: start_date: %w", err)
		}
	}

	for _, market := range cfg.Universe.Markets {
		last, ok, err := store.LastIndexBarDate(ctx, market)
		if err != nil {
			return FetchResult{}, err
		}
		from := nextFrom(start, last, ok)
		if from.After(today) {
			continue
		}
		bars, err := src.IndexBars(ctx, market, from, today)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch: index %s: %w", market, err)
		}
		if err := store.UpsertIndexBars(ctx, market, bars); err != nil {
			return FetchResult{}, fmt.Errorf("fetch: save index %s: %w", market, err)
		}
	}

	syms, err := store.ListSymbols(ctx)
	if err != nil {
		return FetchResult{}, err
	}
	var res FetchResult
	for i, sym := range syms {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		err := fetchSymbol(ctx, store, src, sym.Code, start, today, &res)
		if err != nil {
			res.Failures = append(res.Failures, FetchFailure{Code: sym.Code, Err: err})
		} else {
			res.Symbols++
		}
		if progress != nil {
			progress(FetchProgress{Done: i + 1, Total: len(syms), Code: sym.Code, Err: err})
		}
	}
	return res, nil
}

func fetchSymbol(ctx context.Context, store data.Store, src BarSource, code string, start, today time.Time, res *FetchResult) error {
	last, ok, err := store.LastBarDate(ctx, code)
	if err != nil {
		return err
	}
	from := nextFrom(start, last, ok)
	if from.After(today) {
		return nil
	}
	bars, err := src.DailyBars(ctx, code, from, today)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return nil
	}
	if err := store.UpsertBars(ctx, code, bars); err != nil {
		return err
	}
	res.Bars += len(bars)
	return nil
}

// nextFrom 은 저장된 마지막 날짜가 있으면 그 다음 날, 없으면 start 를 돌려준다.
func nextFrom(start, last time.Time, hasLast bool) time.Time {
	if hasLast {
		return last.AddDate(0, 0, 1)
	}
	return start
}
