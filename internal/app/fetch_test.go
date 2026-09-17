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
	calls       []call
	failCode    string
	unauthCode  string
	indexErr    error
	noIndexBars bool
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
	if f.noIndexBars {
		return nil, nil
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

func TestRunFetchFetchesOnlySymbolsBehindIndex(t *testing.T) {
	ctx := context.Background()
	today := data.Date(2024, 9, 10)
	setupSymbols := func(store *data.SQLiteStore) {
		store.UpsertSymbols(ctx, []data.Symbol{
			{Code: "A", Name: "A", Market: "kospi"},
			{Code: "B", Name: "B", Market: "kospi"},
			{Code: "C", Name: "C", Market: "kospi"},
		})
	}

	// 지수는 무변화(휴장일)지만 종목별로 뒤처진 정도가 다르면, 뒤처진 종목만 호출한다.
	store := openStore(t)
	setupSymbols(store)
	if err := store.UpsertIndexBars(ctx, "kospi", []data.IndexBar{{Date: data.Date(2024, 9, 9), Open: 1, High: 1, Low: 1, Close: 1}}); err != nil {
		t.Fatal(err)
	}
	store.UpsertBars(ctx, "A", []data.Bar{{Date: data.Date(2024, 9, 9), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}}) // A: 지수와 동일 → 호출 없음
	store.UpsertBars(ctx, "C", []data.Bar{{Date: data.Date(2024, 9, 6), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}}) // C: 뒤처짐 → 호출
	// B: 봉 없음 → 호출

	src := &fakeBars{noIndexBars: true}
	var progress []FetchProgress
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today}, func(p FetchProgress) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Symbols != 2 || res.UpToDate != 1 || res.Skipped {
		t.Errorf("result = %+v", res)
	}
	barCalls := map[string]bool{}
	for _, c := range src.calls {
		if c.kind == "bar" {
			barCalls[c.key] = true
		}
	}
	if len(barCalls) != 2 || barCalls["A"] || !barCalls["B"] || !barCalls["C"] {
		t.Errorf("bar calls = %+v", src.calls)
	}
	if len(progress) != 3 || progress[0].Total != 3 || progress[2].Done != 3 {
		t.Errorf("progress = %+v", progress)
	}
	for _, p := range progress {
		if p.Err != nil {
			t.Errorf("progress err = %+v", p)
		}
	}

	// 세 종목 모두 지수와 동일하게 최신이면 전혀 호출하지 않고 Skipped == true.
	store2 := openStore(t)
	setupSymbols(store2)
	if err := store2.UpsertIndexBars(ctx, "kospi", []data.IndexBar{{Date: data.Date(2024, 9, 9), Open: 1, High: 1, Low: 1, Close: 1}}); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"A", "B", "C"} {
		store2.UpsertBars(ctx, code, []data.Bar{{Date: data.Date(2024, 9, 9), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}})
	}
	src2 := &fakeBars{noIndexBars: true}
	res, err = RunFetch(ctx, fetchConfig(), store2, src2, FetchOptions{Today: today}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped || res.UpToDate != 3 {
		t.Errorf("result = %+v", res)
	}
	for _, c := range src2.calls {
		if c.kind == "bar" {
			t.Errorf("no symbol should be called: %+v", src2.calls)
		}
	}

	// 지수에 저장된 봉이 전혀 없으면 latest = today 가 되어, 봉 없는 종목은 모두 호출된다.
	store3 := openStore(t)
	setupSymbols(store3)
	src3 := &fakeBars{}
	res, err = RunFetch(ctx, fetchConfig(), store3, src3, FetchOptions{Today: today}, nil)
	if err != nil {
		t.Fatal(err)
	}
	barCalls3 := map[string]bool{}
	for _, c := range src3.calls {
		if c.kind == "bar" {
			barCalls3[c.key] = true
		}
	}
	if len(barCalls3) != 3 || res.Symbols != 3 || res.UpToDate != 0 {
		t.Errorf("result = %+v, bar calls = %+v", res, src3.calls)
	}
}
