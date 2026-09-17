// Package strategy 는 종가 베팅 전략의 진입 조건(설계 8.2절)을 순수 함수로 제공한다.
// 상태를 갖지 않고 넘겨받은 봉만 본다. 호출자가 당일까지의 봉만 넘겨 미래를 보지 않게 한다.
package strategy

import (
	"math"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

// Market 은 종목이 속한 시장의 지수 일봉. 종목 bars 와 같은 규칙으로 당일까지만 담긴다.
type Market struct {
	IndexBars []data.IndexBar
}

// Turnover 는 봉의 거래대금 근사(종가 × 거래량).
func Turnover(b data.Bar) int64 { return b.Close * b.Volume }

// Entry 는 8.2절 진입 조건 8개를 모두 검사한다. bars 는 과거→현재, 마지막이 당일.
func Entry(cfg config.StrategyConfig, mkt Market, bars []data.Bar) bool {
	return MarketFilter(cfg, mkt) &&
		len(bars) >= cfg.MALongDays+1 &&
		Liquidity(cfg, bars) &&
		CloseStrength(cfg, bars) &&
		Change(cfg, bars) &&
		Trend(cfg, bars) &&
		NewHigh(cfg, bars)
}

// MarketFilter (조건 1): 지수 봉이 index_ma_days 개 이상이고 당일 종가 > index_ma_days 일 평균(당일 포함).
func MarketFilter(cfg config.StrategyConfig, mkt Market) bool {
	n := cfg.IndexMADays
	b := mkt.IndexBars
	if n <= 0 || len(b) < n {
		return false
	}
	sum := 0.0
	for _, x := range b[len(b)-n:] {
		sum += x.Close
	}
	return b[len(b)-1].Close > sum/float64(n)
}

// Liquidity (조건 3·4): 당일 거래대금 ≥ min_turnover 이고 ≥ 직전 turnover_ma_days 일 평균(당일 제외) × ratio.
func Liquidity(cfg config.StrategyConfig, bars []data.Bar) bool {
	k := cfg.TurnoverMADays
	if k <= 0 || len(bars) < k+1 {
		return false
	}
	today := float64(Turnover(bars[len(bars)-1]))
	if today < float64(cfg.MinTurnover) {
		return false
	}
	return today >= avgTurnover(bars[len(bars)-1-k:len(bars)-1])*cfg.TurnoverRatioMin
}

// CloseStrength (조건 5): 종가 ≥ 고가 × close_to_high_min.
func CloseStrength(cfg config.StrategyConfig, bars []data.Bar) bool {
	if len(bars) == 0 {
		return false
	}
	b := bars[len(bars)-1]
	return float64(b.Close) >= float64(b.High)*cfg.CloseToHighMin
}

// MinClose 는 change_min 을 만족하는 최소 종가(정수). 부동소수 오차로 1원 튀지 않게 작은 값을 뺀 뒤 올린다.
func MinClose(prevClose int64, changeMin float64) int64 {
	return int64(math.Ceil(float64(prevClose)*(1+changeMin) - 1e-6))
}

// Change (조건 6): change_min ≤ 종가/전일 종가 − 1 < change_max.
func Change(cfg config.StrategyConfig, bars []data.Bar) bool {
	if len(bars) < 2 || bars[len(bars)-2].Close <= 0 {
		return false
	}
	c := bars[len(bars)-1].Close
	p := bars[len(bars)-2].Close
	return c >= MinClose(p, cfg.ChangeMin) && c < MinClose(p, cfg.ChangeMax)
}

// Trend (조건 7): 종가 > 단기 MA > 장기 MA (당일 포함). 봉이 ma_long_days 개 미만이면 false.
func Trend(cfg config.StrategyConfig, bars []data.Bar) bool {
	n, m := cfg.MAShortDays, cfg.MALongDays
	if n <= 0 || m <= n || len(bars) < m {
		return false
	}
	short, long := maClose(bars, n), maClose(bars, m)
	return float64(bars[len(bars)-1].Close) > short && short > long
}

// NewHigh (조건 8): 종가 ≥ 직전 new_high_days 일(당일 제외) 고가 최댓값.
func NewHigh(cfg config.StrategyConfig, bars []data.Bar) bool {
	k := cfg.NewHighDays
	if k <= 0 || len(bars) < k+1 {
		return false
	}
	return bars[len(bars)-1].Close >= maxHigh(bars[len(bars)-1-k:len(bars)-1])
}

// Score 는 후보 우선순위: 당일 거래대금 / 직전 turnover_ma_days 일 평균. 봉이 부족하면 0.
func Score(cfg config.StrategyConfig, bars []data.Bar) float64 {
	k := cfg.TurnoverMADays
	if k <= 0 || len(bars) < k+1 {
		return 0
	}
	avg := avgTurnover(bars[len(bars)-1-k : len(bars)-1])
	if avg == 0 {
		return 0
	}
	return float64(Turnover(bars[len(bars)-1])) / avg
}

// maClose 는 마지막 n 개 종가의 평균.
func maClose(bars []data.Bar, n int) float64 {
	sum := int64(0)
	for _, b := range bars[len(bars)-n:] {
		sum += b.Close
	}
	return float64(sum) / float64(n)
}

func avgTurnover(bars []data.Bar) float64 {
	if len(bars) == 0 {
		return 0
	}
	sum := int64(0)
	for _, b := range bars {
		sum += Turnover(b)
	}
	return float64(sum) / float64(len(bars))
}

func maxHigh(bars []data.Bar) int64 {
	var h int64
	for _, b := range bars {
		if b.High > h {
			h = b.High
		}
	}
	return h
}
