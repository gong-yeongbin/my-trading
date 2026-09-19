package data

import (
	"context"
	"time"
)

type Store interface {
	UpsertSymbols(ctx context.Context, syms []Symbol) error
	ListSymbols(ctx context.Context) ([]Symbol, error)
	DeleteSymbols(ctx context.Context, codes []string) error // 봉 이력은 남긴다

	LastBarDate(ctx context.Context, code string) (time.Time, bool, error)
	UpsertBars(ctx context.Context, code string, bars []Bar) error
	LoadBars(ctx context.Context, code string, from, to time.Time) ([]Bar, error)

	LastIndexBarDate(ctx context.Context, market string) (time.Time, bool, error)
	UpsertIndexBars(ctx context.Context, market string, bars []IndexBar) error
	LoadIndexBars(ctx context.Context, market string, from, to time.Time) ([]IndexBar, error)
}
