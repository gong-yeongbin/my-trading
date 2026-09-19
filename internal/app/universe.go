// Package app 은 서브커맨드와 TUI 가 공유하는 실행 로직이다.
package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
)

type MasterSource interface {
	DownloadMaster(ctx context.Context, market string) ([]kis.MasterRow, error)
}

type UniverseResult struct {
	Downloaded int
	Kept       int
	Dropped    int // 마스터에는 있지만 이번에 제외돼 symbols 에서 지운 종목
	ByMarket   map[string]int
}

// RunUniverse 는 마스터 파일을 받아 그룹코드·제외 플래그로 거른 종목을 symbols 테이블에 저장한다.
// 기존 종목은 갱신되고, 마스터에 있지만 이번에 제외된 종목은 symbols 에서 지운다(봉은 남긴다 — 다시 들어오면 fetch 가 빈 구간을 채운다).
// 마스터에서 사라진 종목은 건드리지 않는다.
func RunUniverse(ctx context.Context, cfg *config.Config, store data.Store, src MasterSource) (UniverseResult, error) {
	for _, flag := range cfg.Universe.ExcludeFlags {
		if _, err := (kis.MasterRow{}).FlagOn(flag); err != nil {
			return UniverseResult{}, fmt.Errorf("universe.exclude_flags: %w", err)
		}
	}
	res := UniverseResult{ByMarket: map[string]int{}}
	var keep []data.Symbol
	var drop []string
	for _, market := range cfg.Universe.Markets {
		rows, err := src.DownloadMaster(ctx, market)
		if err != nil {
			return UniverseResult{}, fmt.Errorf("universe: %s: %w", market, err)
		}
		res.Downloaded += len(rows)
		for _, r := range rows {
			if !slices.Contains(cfg.Universe.GroupCodes, r.GroupCode) || excluded(r, cfg.Universe.ExcludeFlags) {
				drop = append(drop, r.Code)
				continue
			}
			keep = append(keep, data.Symbol{Code: r.Code, Name: r.Name, Market: r.Market})
			res.ByMarket[market]++
		}
	}
	if err := store.UpsertSymbols(ctx, keep); err != nil {
		return UniverseResult{}, fmt.Errorf("universe: save: %w", err)
	}
	existing, err := store.ListSymbols(ctx)
	if err != nil {
		return UniverseResult{}, err
	}
	have := map[string]bool{}
	for _, s := range existing {
		have[s.Code] = true
	}
	drop = slices.DeleteFunc(drop, func(code string) bool { return !have[code] })
	if err := store.DeleteSymbols(ctx, drop); err != nil {
		return UniverseResult{}, fmt.Errorf("universe: drop: %w", err)
	}
	res.Kept = len(keep)
	res.Dropped = len(drop)
	return res, nil
}

func excluded(r kis.MasterRow, flags []string) bool {
	for _, flag := range flags {
		if on, _ := r.FlagOn(flag); on {
			return true
		}
	}
	return false
}
