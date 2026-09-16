package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
)

type call struct {
	kind, key string
	from, to  time.Time
}

type fakeBars struct {
	calls      []call
	failCode   string
	unauthCode string
	indexErr   error
}

func (f *fakeBars) DailyBars(_ context.Context, code string, from, to time.Time) ([]data.Bar, error) {
	f.calls = append(f.calls, call{"bar", code, from, to})
	if code == f.unauthCode {
		return nil, fmt.Errorf("x: %w", kis.ErrUnauthorized)
	}
	if code == f.failCode {
		return nil, errors.New("boom")
	}
	var out []data.Bar
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, data.Bar{Date: d, Open: 1, High: 2, Low: 1, Close: 2, Volume: 3})
	}
	return out, nil
}

func (f *fakeBars) IndexBars(_ context.Context, market string, from, to time.Time) ([]data.IndexBar, error) {
	f.calls = append(f.calls, call{"index", market, from, to})
	if f.indexErr != nil {
		return nil, f.indexErr
	}
	var out []data.IndexBar
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, data.IndexBar{Date: d, Open: 1, High: 2, Low: 1, Close: 2})
	}
	return out, nil
}

func fetchConfig() *config.Config {
	return &config.Config{
		Universe: config.UniverseConfig{Markets: []string{"kospi"}},
		Fetch:    config.FetchConfig{StartDate: "2024-09-01"},
	}
}

func TestRunFetchInitialAndIncremental(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "005930", Name: "삼성전자", Market: "kospi"}, {Code: "000660", Name: "SK하이닉스", Market: "kospi"}})
	src := &fakeBars{}
	today := data.Date(2024, 9, 10)
	var progress []FetchProgress
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today}, func(p FetchProgress) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}
	// 지수 먼저, 그 다음 종목 순서
	if len(src.calls) != 3 || src.calls[0].kind != "index" || !src.calls[0].from.Equal(data.Date(2024, 9, 1)) || !src.calls[0].to.Equal(today) {
		t.Fatalf("calls = %+v", src.calls)
	}
	if src.calls[1].key != "000660" || src.calls[2].key != "005930" || !src.calls[1].from.Equal(data.Date(2024, 9, 1)) {
		t.Errorf("symbol calls = %+v", src.calls[1:])
	}
	if res.Symbols != 2 || res.Bars != 20 || len(res.Failures) != 0 {
		t.Errorf("result = %+v", res)
	}
	if len(progress) != 2 || progress[1].Done != 2 || progress[1].Total != 2 {
		t.Errorf("progress = %+v", progress)
	}
	if last, ok, _ := store.LastIndexBarDate(ctx, "kospi"); !ok || !last.Equal(today) {
		t.Errorf("index not stored: %v %v", last, ok)
	}

	// 증분: 마지막 저장일 다음 날부터
	src.calls = nil
	today2 := data.Date(2024, 9, 12)
	if _, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today2}, nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range src.calls {
		if !c.from.Equal(data.Date(2024, 9, 11)) || !c.to.Equal(today2) {
			t.Errorf("incremental range wrong: %+v", c)
		}
	}
	// 이미 오늘까지 있으면 호출하지 않는다
	src.calls = nil
	if _, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today2}, nil); err != nil {
		t.Fatal(err)
	}
	if len(src.calls) != 0 {
		t.Errorf("expected no calls when up to date, got %+v", src.calls)
	}
}

func TestRunFetchFromOverridesStartOnlyWhenEmpty(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "005930", Name: "삼성전자", Market: "kospi"}})
	store.UpsertBars(ctx, "005930", []data.Bar{{Date: data.Date(2024, 9, 5), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}})
	src := &fakeBars{}
	_, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{From: data.Date(2024, 8, 1), Today: data.Date(2024, 9, 10)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !src.calls[0].from.Equal(data.Date(2024, 8, 1)) { // 지수는 비어 있으므로 --from
		t.Errorf("index from = %v", src.calls[0].from)
	}
	if !src.calls[1].from.Equal(data.Date(2024, 9, 6)) { // 종목은 저장분 다음 날
		t.Errorf("symbol from = %v", src.calls[1].from)
	}
}

func TestRunFetchContinuesOnSymbolFailure(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "000660", Name: "SK하이닉스", Market: "kospi"}, {Code: "005930", Name: "삼성전자", Market: "kospi"}})
	src := &fakeBars{failCode: "000660"}
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: data.Date(2024, 9, 2)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failures) != 1 || res.Failures[0].Code != "000660" || res.Symbols != 1 {
		t.Errorf("result = %+v", res)
	}
	if _, ok, _ := store.LastBarDate(ctx, "005930"); !ok {
		t.Error("healthy symbol should still be stored")
	}
}

func TestRunFetchAbortsOnUnauthorized(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "000660", Name: "SK하이닉스", Market: "kospi"}, {Code: "005930", Name: "삼성전자", Market: "kospi"}})
	src := &fakeBars{unauthCode: "000660"}
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: data.Date(2024, 9, 2)}, nil)
	if !errors.Is(err, kis.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	// 지수(1) + 첫 종목(1) 이후 중단 — 두 번째 종목은 호출되지 않는다
	if len(src.calls) != 2 {
		t.Errorf("expected no further symbol calls after abort, got %+v", src.calls)
	}
	if len(res.Failures) > 1 {
		t.Errorf("expected 0 or 1 recorded failures, got %+v", res.Failures)
	}
}

func TestRunFetchStopsOnIndexFailure(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{Code: "005930", Name: "삼성전자", Market: "kospi"}})
	src := &fakeBars{indexErr: errors.New("index down")}
	if _, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: data.Date(2024, 9, 2)}, nil); err == nil {
		t.Fatal("expected error")
	}
	if len(src.calls) != 1 {
		t.Errorf("symbols must not be fetched after index failure: %+v", src.calls)
	}
}
