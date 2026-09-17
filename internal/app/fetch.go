package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
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
	Symbols  int // 새로 호출해 성공한 종목 수
	Bars     int // 저장한 봉 수
	Failures []FetchFailure
	UpToDate int  // 지수 기준으로 이미 최신이라 호출을 건너뛴 종목 수
	Skipped  bool // 종목 API 호출이 하나도 없었음
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

	indexNew := 0
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
		indexNew += len(bars)
	}

	// latest: 모든 시장의 지수 마지막 봉 날짜 중 최댓값. 봉이 하나도 없으면 today.
	var latest time.Time
	for _, market := range cfg.Universe.Markets {
		last, ok, err := store.LastIndexBarDate(ctx, market)
		if err != nil {
			return FetchResult{}, err
		}
		if ok && (latest.IsZero() || last.After(latest)) {
			latest = last
		}
	}
	if latest.IsZero() {
		latest = today
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
		upToDate, err := fetchSymbol(ctx, store, src, sym.Code, start, today, latest, &res)
		if err != nil {
			if errors.Is(err, kis.ErrUnauthorized) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return res, err
			}
			res.Failures = append(res.Failures, FetchFailure{Code: sym.Code, Err: err})
		} else if !upToDate {
			res.Symbols++
		}
		if progress != nil {
			progress(FetchProgress{Done: i + 1, Total: len(syms), Code: sym.Code, Err: err})
		}
	}
	res.Skipped = len(syms) > 0 && res.UpToDate == len(syms)
	return res, nil
}

// fetchSymbol 은 종목의 마지막 저장일이 latest(지수 기준 최신일) 이상이면 호출 없이 건너뛰고
// (upToDate=true, res.UpToDate++), 아니면 증분 수집한다.
func fetchSymbol(ctx context.Context, store data.Store, src BarSource, code string, start, today, latest time.Time, res *FetchResult) (upToDate bool, err error) {
	last, ok, err := store.LastBarDate(ctx, code)
	if err != nil {
		return false, err
	}
	if ok && !last.Before(latest) {
		res.UpToDate++
		return true, nil
	}
	from := nextFrom(start, last, ok)
	if from.After(today) {
		return false, nil
	}
	bars, err := src.DailyBars(ctx, code, from, today)
	if err != nil {
		return false, err
	}
	if len(bars) == 0 {
		return false, nil
	}
	if err := store.UpsertBars(ctx, code, bars); err != nil {
		return false, err
	}
	res.Bars += len(bars)
	return false, nil
}

// nextFrom 은 저장된 마지막 날짜가 있으면 그 다음 날, 없으면 start 를 돌려준다.
func nextFrom(start, last time.Time, hasLast bool) time.Time {
	if hasLast {
		return last.AddDate(0, 0, 1)
	}
	return start
}
