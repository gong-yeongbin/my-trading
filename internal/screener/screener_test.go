package screener

import (
	"context"
	"math"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/strategy"
)

func cfg() config.StrategyConfig {
	return config.StrategyConfig{
		IndexMADays: 20, MinTurnover: 10_000_000_000, TurnoverMADays: 20, TurnoverRatioMin: 3.0,
		CloseToHighMin: 0.99, ChangeMin: 0.03, ChangeMax: 0.20, MAShortDays: 20, MALongDays: 60, NewHighDays: 20,
	}
}

// randomBars 는 무작위 걸음 봉 n 개. 가격은 1,000~100,000 원, 거래량 10만~200만.
func randomBars(r *rand.Rand, n int) []data.Bar {
	out := make([]data.Bar, n)
	price := int64(1000 + r.Intn(50_000))
	for i := range out {
		price += int64(r.Intn(2001) - 1000)
		if price < 1000 {
			price = 1000
		}
		high := price + int64(r.Intn(500))
		low := price - int64(r.Intn(500))
		if low < 1 {
			low = 1
		}
		out[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: price, High: high, Low: low, Close: price, Volume: int64(100_000 + r.Intn(1_900_000))}
	}
	return out
}

func sym() data.Symbol { return data.Symbol{Code: "000001", Name: "테스트", Market: "kospi"} }

// TestThresholdsMatchStrategy 는 9.3절: C = C_min 이면 가격 조건(6·7·8)과 유동성이 통과하고, C = C_min−1 이면 가격 조건 중 하나가 실패한다.
func TestThresholdsMatchStrategy(t *testing.T) {
	t.Run("default", func(t *testing.T) { checkThresholdsMatchStrategy(t, cfg(), 42) })
	changeMin10 := cfg()
	changeMin10.ChangeMin = 0.1
	t.Run("change_min_0.1", func(t *testing.T) { checkThresholdsMatchStrategy(t, changeMin10, 43) })
}

// checkThresholdsMatchStrategy 는 9.3절: C = C_min 이면 가격 조건(6·7·8)과 유동성이 통과하고, C = C_min−1 이면 가격 조건 중 하나가 실패한다.
func checkThresholdsMatchStrategy(t *testing.T, c config.StrategyConfig, seed int64) {
	r := rand.New(rand.NewSource(seed))
	feasible := 0
	for i := 0; i < 500; i++ {
		prev := randomBars(r, 60+r.Intn(30))
		item, ok := Compute(c, sym(), prev)
		if !ok {
			continue
		}
		feasible++
		next := prev[len(prev)-1].Date.AddDate(0, 0, 1)
		pass := append(append([]data.Bar(nil), prev...), data.Bar{Date: next, Open: item.MinClose, High: item.MinClose, Low: item.MinClose, Close: item.MinClose, Volume: item.MinVolume})
		if !strategy.Change(c, pass) || !strategy.Trend(c, pass) || !strategy.NewHigh(c, pass) || !strategy.Liquidity(c, pass) || !strategy.CloseStrength(c, pass) {
			t.Fatalf("case %d: C=C_min should pass: change=%v trend=%v newhigh=%v liq=%v item=%+v", i,
				strategy.Change(c, pass), strategy.Trend(c, pass), strategy.NewHigh(c, pass), strategy.Liquidity(c, pass), item)
		}
		fail := append(append([]data.Bar(nil), prev...), data.Bar{Date: next, Open: item.MinClose - 1, High: item.MinClose - 1, Low: item.MinClose - 1, Close: item.MinClose - 1, Volume: item.MinVolume * 10})
		if strategy.Change(c, fail) && strategy.Trend(c, fail) && strategy.NewHigh(c, fail) {
			t.Fatalf("case %d: C=C_min-1 should fail a price condition: item=%+v", i, item)
		}
		if item.PrevClose != prev[len(prev)-1].Close || item.MinChangePct != float64(item.MinClose)/float64(item.PrevClose)-1 {
			t.Fatalf("case %d: derived fields wrong: %+v", i, item)
		}
	}
	if feasible < 20 {
		t.Fatalf("too few feasible cases to trust the test: %d", feasible)
	}
	t.Logf("feasible cases: %d", feasible)
}

func TestComputeRejectsShortHistoryAndInfeasible(t *testing.T) {
	c := cfg()
	r := rand.New(rand.NewSource(7))
	if _, ok := Compute(c, sym(), randomBars(r, 59)); ok {
		t.Error("fewer than ma_long_days bars must be rejected")
	}
	// 직전 20일 고가가 전일 종가의 1.5배면 상승률 상한(20%) 안에서 신고가를 못 넘긴다 → 실현 불가
	bars := randomBars(r, 60)
	last := bars[len(bars)-1].Close
	bars[len(bars)-5].High = last * 3 / 2
	if _, ok := Compute(c, sym(), bars); ok {
		t.Error("infeasible (new high beyond change_max) must be rejected")
	}
}

func TestComputeThresholdValues(t *testing.T) {
	c := cfg()
	// 60개 모두 종가 10,000·고가 10,000·거래량 100만(거래대금 100억). 단기·장기선 하한 = 10,001, 상승률 하한 = 10,300, 신고가 = 10,000 → C_min 10,300
	bars := make([]data.Bar, 60)
	for i := range bars {
		bars[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: 10_000, High: 10_000, Low: 10_000, Close: 10_000, Volume: 1_000_000}
	}
	item, ok := Compute(c, sym(), bars)
	if !ok {
		t.Fatal("expected feasible")
	}
	if item.MinClose != 10_300 || item.Thresholds["change_min"] != 10_300 || item.Thresholds["new_high"] != 10_000 || item.Thresholds["ma_short"] != 10_001 || item.Thresholds["ma_long"] != 10_001 {
		t.Errorf("thresholds = %+v", item)
	}
	// T = 100억, ratio 3 → T_req 300억, MinVolume = ceil(300억 / 10,300) = 2,912,622
	if item.MinTurnover != 30_000_000_000 || item.MinVolume != int64(math.Ceil(30_000_000_000.0/10_300)) {
		t.Errorf("turnover thresholds = %+v", item)
	}
	if math.Abs(item.MinChangePct-0.03) > 1e-9 {
		t.Errorf("MinChangePct = %v", item.MinChangePct)
	}
}

func TestFilterStatus(t *testing.T) {
	c := cfg()
	mk := func(n int, last float64) []data.IndexBar {
		out := make([]data.IndexBar, n)
		for i := range out {
			out[i] = data.IndexBar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Close: 1000}
		}
		if n > 0 {
			out[n-1].Close = last
		}
		return out
	}
	if s := FilterStatus(c, mk(20, 1001)); s != "진입가능" {
		t.Errorf("above ma: %q", s)
	}
	if s := FilterStatus(c, mk(20, 1000)); s != "차단" {
		t.Errorf("on ma: %q", s)
	}
	if s := FilterStatus(c, mk(19, 1001)); s != "알 수 없음" {
		t.Errorf("short: %q", s)
	}
}

func TestRunSortsAndUsesIndexDate(t *testing.T) {
	ctx := context.Background()
	store, err := data.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := &config.Config{Universe: config.UniverseConfig{Markets: []string{"kospi", "kosdaq"}}, Strategy: cfg()}
	if err := store.UpsertSymbols(ctx, []data.Symbol{{Code: "B", Name: "비", Market: "kospi"}, {Code: "A", Name: "에이", Market: "kosdaq"}, {Code: "S", Name: "짧음", Market: "kospi"}, {Code: "O", Name: "옛날", Market: "kospi"}}); err != nil {
		t.Fatal(err)
	}
	flat := func(n int, close, vol int64, extra int) []data.Bar {
		out := make([]data.Bar, n+extra)
		for i := range out {
			out[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close, Volume: vol}
		}
		return out
	}
	// B: 60개 + 지수 이후 날짜 1개(잘려야 함, 값이 다름) ; A: 60개, 마지막 종가 더 낮은 문턱 ; S: 30개
	bB := flat(60, 10_000, 1_000_000, 1)
	bB[60].Close = 99_999 // 지수 마지막 날짜 뒤 → 무시되어야 함
	if err := store.UpsertBars(ctx, "B", bB); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertBars(ctx, "A", flat(60, 10_000, 1_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertBars(ctx, "S", flat(30, 10_000, 1_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	// O: 60개, 기준일(day 59)보다 3일 앞선 day 56 에서 끝남 → 기준일 봉 없음, 제외돼야 함.
	bO := make([]data.Bar, 60)
	for i := range bO {
		bO[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, -3+i), Open: 10_000, High: 10_000, Low: 10_000, Close: 10_000, Volume: 1_000_000}
	}
	if err := store.UpsertBars(ctx, "O", bO); err != nil {
		t.Fatal(err)
	}
	idx := make([]data.IndexBar, 60)
	for i := range idx {
		idx[i] = data.IndexBar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Close: 1000}
	}
	idx[59].Close = 1001
	if err := store.UpsertIndexBars(ctx, "kospi", idx); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertIndexBars(ctx, "kosdaq", idx[:10]); err != nil {
		t.Fatal(err)
	}

	res, err := Run(ctx, c, store)
	if err != nil {
		t.Fatal(err)
	}
	if !res.AsOf.Equal(data.Date(2026, 1, 1).AddDate(0, 0, 59)) {
		t.Errorf("AsOf = %v", res.AsOf)
	}
	if len(res.Items) != 2 || res.Items[0].Code != "A" || res.Items[1].Code != "B" {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[1].PrevClose != 10_000 {
		t.Errorf("B should ignore bars after index date: %+v", res.Items[1])
	}
	if res.Filter["kospi"] != "진입가능" || res.Filter["kosdaq"] != "알 수 없음" {
		t.Errorf("filter = %v", res.Filter)
	}
}

func TestRunWithoutIndexReturnsEmpty(t *testing.T) {
	store, err := data.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := &config.Config{Universe: config.UniverseConfig{Markets: []string{"kospi"}}, Strategy: cfg()}
	res, err := Run(context.Background(), c, store)
	if err != nil || len(res.Items) != 0 || !res.AsOf.IsZero() {
		t.Errorf("empty store: %+v %v", res, err)
	}
}
