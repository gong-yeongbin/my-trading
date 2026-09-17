package strategy

import (
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

func cfg() config.StrategyConfig {
	return config.StrategyConfig{
		IndexMADays: 20, MinTurnover: 10_000_000_000, TurnoverMADays: 20, TurnoverRatioMin: 3.0,
		CloseToHighMin: 0.99, ChangeMin: 0.03, ChangeMax: 0.20, MAShortDays: 20, MALongDays: 60, NewHighDays: 20,
	}
}

// flat 은 n 개의 같은 봉 (종가 close, 고가 close, 거래량 vol).
func flat(n int, close, vol int64) []data.Bar {
	out := make([]data.Bar, n)
	for i := range out {
		out[i] = data.Bar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close, Volume: vol}
	}
	return out
}

func withLast(bars []data.Bar, last data.Bar) []data.Bar {
	out := append([]data.Bar(nil), bars...)
	last.Date = out[len(out)-1].Date.AddDate(0, 0, 1)
	return append(out, last)
}

func indexFlat(n int, close float64) Market {
	m := Market{}
	for i := 0; i < n; i++ {
		m.IndexBars = append(m.IndexBars, data.IndexBar{Date: data.Date(2026, 1, 1).AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close})
	}
	return m
}

func TestMarketFilterBoundary(t *testing.T) {
	c := cfg()
	// 19일 평균 1000, 당일 1000 → 20일 평균 1000, 종가 == 평균 → 차단
	m := indexFlat(20, 1000)
	if MarketFilter(c, m) {
		t.Error("close == ma should fail")
	}
	m.IndexBars[19].Close = 1000.01 // 평균 1000.0005, 종가 > 평균
	if !MarketFilter(c, m) {
		t.Error("close > ma should pass")
	}
	if MarketFilter(c, indexFlat(19, 1000)) {
		t.Error("fewer than index_ma_days bars should fail")
	}
}

func TestLiquidityBoundary(t *testing.T) {
	c := cfg()
	base := flat(20, 10_000, 100_000) // 직전 20일 거래대금 10억
	// 당일 거래대금 = 30억 = 3배 정확히, 그러나 100억 미만 → 실패
	if Liquidity(c, withLast(base, data.Bar{Close: 10_000, High: 10_000, Volume: 300_000})) {
		t.Error("below min_turnover should fail")
	}
	big := flat(20, 10_000, 500_000) // 직전 평균 50억
	// 당일 150억 = 정확히 3배, 100억 이상 → 통과
	if !Liquidity(c, withLast(big, data.Bar{Close: 10_000, High: 10_000, Volume: 1_500_000})) {
		t.Error("exactly 3x and >= min_turnover should pass")
	}
	if Liquidity(c, withLast(big, data.Bar{Close: 10_000, High: 10_000, Volume: 1_499_999})) {
		t.Error("just under 3x should fail")
	}
	if Liquidity(c, flat(20, 10_000, 1_500_000)) { // 직전 봉 19개뿐
		t.Error("fewer than turnover_ma_days+1 bars should fail")
	}
}

func TestCloseStrengthBoundary(t *testing.T) {
	c := cfg()
	if !CloseStrength(c, []data.Bar{{Close: 9_900, High: 10_000}}) {
		t.Error("exactly 0.99 should pass")
	}
	if CloseStrength(c, []data.Bar{{Close: 9_899, High: 10_000}}) {
		t.Error("below 0.99 should fail")
	}
	if CloseStrength(c, nil) {
		t.Error("no bars should fail")
	}
}

func TestChangeBoundary(t *testing.T) {
	c := cfg()
	prev := flat(1, 10_000, 1)
	for _, tc := range []struct {
		close int64
		want  bool
	}{{10_300, true}, {10_299, false}, {11_999, true}, {12_000, false}} {
		if got := Change(c, withLast(prev, data.Bar{Close: tc.close})); got != tc.want {
			t.Errorf("close %d: Change = %v, want %v", tc.close, got, tc.want)
		}
	}
	if Change(c, prev) {
		t.Error("single bar should fail")
	}

	c10 := c
	c10.ChangeMin = 0.1
	if !Change(c10, withLast(prev, data.Bar{Close: 11_000})) {
		t.Error("change_min 0.1: close 11000 should pass")
	}
	if Change(c10, withLast(prev, data.Bar{Close: 10_999})) {
		t.Error("change_min 0.1: close 10999 should fail")
	}

	cMax := c
	cMax.ChangeMax = 0.1
	prev50 := flat(1, 50, 1)
	if Change(cMax, withLast(prev50, data.Bar{Close: 55})) {
		t.Error("change_max 0.1: close 55 (== 50*1.1 boundary) should fail")
	}
	if !Change(cMax, withLast(prev50, data.Bar{Close: 54})) {
		t.Error("change_max 0.1: close 54 should pass")
	}
}

func TestMinCloseAvoidsFloatDrift(t *testing.T) {
	for _, tc := range []struct {
		prev      int64
		changeMin float64
		want      int64
	}{
		{10_000, 0.1, 11_000},
		{1_900, 0.07, 2_033},
		{10_000, 0.03, 10_300},
		{12_345, 0.03, 12_716},
		{50, 0.1, 55},
	} {
		if got := MinClose(tc.prev, tc.changeMin); got != tc.want {
			t.Errorf("MinClose(%d, %v) = %d, want %d", tc.prev, tc.changeMin, got, tc.want)
		}
	}
}

func TestTrendBoundary(t *testing.T) {
	c := cfg()
	// 59개 1000, 당일 1000 → 20일선 == 60일선 == 종가 → 실패
	if Trend(c, flat(60, 1000, 1)) {
		t.Error("all equal should fail")
	}
	// 40개 900 + 19개 1000 + 당일 1001: 20일선 (19000+1001)/20=1000.05, 60일선 (36000+19000+1001)/60=933.35 → 통과
	bars := append(flat(40, 900, 1), flat(19, 1000, 1)...)
	if !Trend(c, withLast(bars, data.Bar{Close: 1001})) {
		t.Error("close > ma20 > ma60 should pass")
	}
	// 당일 1000: 20일선 = 1000 == 종가 → 실패
	if Trend(c, withLast(bars, data.Bar{Close: 1000})) {
		t.Error("close == ma20 should fail")
	}
	if Trend(c, flat(59, 1000, 1)) {
		t.Error("fewer than ma_long_days bars should fail")
	}
}

func TestNewHighBoundary(t *testing.T) {
	c := cfg()
	prev := flat(20, 1000, 1)
	prev[5].High = 1500
	if !NewHigh(c, withLast(prev, data.Bar{Close: 1500, High: 1600})) {
		t.Error("close == prior 20d high should pass")
	}
	if NewHigh(c, withLast(prev, data.Bar{Close: 1499, High: 1600})) {
		t.Error("close below prior high should fail")
	}
	if NewHigh(c, withLast(flat(19, 1000, 1), data.Bar{Close: 2000})) {
		t.Error("fewer than new_high_days prior bars should fail")
	}
}

func TestEntryRequiresAll(t *testing.T) {
	c := cfg()
	// 통과하는 케이스를 만든다: 직전 60일 900/거래량 100만(거래대금 9억), 당일 종가 1000·고가 1000·거래량 1500만(150억) → 총 61개
	prev := flat(60, 900, 1_000_000)
	last := data.Bar{Open: 950, High: 1000, Low: 940, Close: 1000, Volume: 15_000_000}
	bars := withLast(prev, last)
	m := indexFlat(20, 1000)
	m.IndexBars[19].Close = 1001
	if !Entry(c, m, bars) {
		t.Fatalf("expected entry: mf=%v liq=%v cs=%v ch=%v tr=%v nh=%v", MarketFilter(c, m), Liquidity(c, bars), CloseStrength(c, bars), Change(c, bars), Trend(c, bars), NewHigh(c, bars))
	}
	// 조건을 하나씩 깨뜨린다
	bad := indexFlat(20, 1000)
	if Entry(c, bad, bars) {
		t.Error("market filter off should block")
	}
	weak := withLast(prev, data.Bar{Open: 950, High: 1020, Low: 940, Close: 1000, Volume: 15_000_000}) // 1000/1020 < 0.99
	if Entry(c, m, weak) {
		t.Error("close strength off should block")
	}
	if Entry(c, m, withLast(flat(60, 900, 1_000_000), data.Bar{High: 1000, Close: 1000, Volume: 9_999_999})) { // 99.99억
		t.Error("liquidity off should block")
	}
	if Entry(c, m, bars[1:]) { // 60개: ma_long_days+1 미만
		t.Error("too few bars should block")
	}
}

func TestScore(t *testing.T) {
	c := cfg()
	bars := withLast(flat(20, 1000, 1000), data.Bar{Close: 1000, Volume: 3000})
	if s := Score(c, bars); s < 2.999 || s > 3.001 {
		t.Errorf("score = %v, want 3", s)
	}
	if Score(c, flat(5, 1000, 1)) != 0 {
		t.Error("too few bars should score 0")
	}
}
