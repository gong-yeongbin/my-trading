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
	ByMarket   map[string]int
}

// RunUniverse 는 마스터 파일을 받아 그룹코드·제외 플래그로 거른 종목을 symbols 테이블에 저장한다.
// 기존 종목은 갱신되고, 마스터에서 사라진 종목은 삭제하지 않는다 (봉 이력 보존).
func RunUniverse(ctx context.Context, cfg *config.Config, store data.Store, src MasterSource) (UniverseResult, error) {
	for _, flag := range cfg.Universe.ExcludeFlags {
		if _, err := (kis.MasterRow{}).FlagOn(flag); err != nil {
			return UniverseResult{}, fmt.Errorf("universe.exclude_flags: %w", err)
		}
	}
	res := UniverseResult{ByMarket: map[string]int{}}
	var keep []data.Symbol
	for _, market := range cfg.Universe.Markets {
		rows, err := src.DownloadMaster(ctx, market)
		if err != nil {
			return UniverseResult{}, fmt.Errorf("universe: %s: %w", market, err)
		}
		res.Downloaded += len(rows)
		for _, r := range rows {
			if !slices.Contains(cfg.Universe.GroupCodes, r.GroupCode) {
				continue
			}
			if excluded(r, cfg.Universe.ExcludeFlags) {
				continue
			}
			keep = append(keep, data.Symbol{Code: r.Code, Name: r.Name, Market: r.Market})
			res.ByMarket[market]++
		}
	}
	if err := store.UpsertSymbols(ctx, keep); err != nil {
		return UniverseResult{}, fmt.Errorf("universe: save: %w", err)
	}
	res.Kept = len(keep)
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
