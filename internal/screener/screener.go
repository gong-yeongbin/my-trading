// Package screener 는 전일까지의 일봉으로 "오늘 종가가 얼마 이상이면 진입 조건을 통과하는가"(설계 9절)를 종목마다 계산한다.
// 네트워크를 쓰지 않는다.
package screener

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/strategy"
)

// WatchItem 은 관심 종목 한 줄.
type WatchItem struct {
	Code, Name, Market string
	PrevClose          int64
	MinClose           int64            // C_min: 오늘 종가 하한
	MinChangePct       float64          // C_min / P − 1
	MinTurnover        int64            // T_req
	MinVolume          int64            // T_req / C_min (올림)
	AvgTurnover        int64            // 최근 turnover_ma_days 평균 거래대금 (정렬키)
	Thresholds         map[string]int64 // 조건별 하한: change_min, new_high, ma_short, ma_long
}

// Compute 는 전일까지의 봉(과거→현재)으로 문턱값을 계산한다. 봉이 ma_long_days 개 미만이거나, 전일 종가가 min_price 미만이거나,
// 실현 불가능하면 false.
func Compute(cfg config.StrategyConfig, sym data.Symbol, bars []data.Bar) (WatchItem, bool) {
	n, m, k, h := cfg.MAShortDays, cfg.MALongDays, cfg.TurnoverMADays, cfg.NewHighDays
	if n <= 1 || m <= n || k <= 0 || h <= 0 || len(bars) < m || len(bars) < k || len(bars) < h {
		return WatchItem{}, false
	}
	p := bars[len(bars)-1].Close
	if p <= 0 || p < cfg.MinPrice {
		return WatchItem{}, false
	}
	sumN, sumM := sumClose(bars, n-1), sumClose(bars, m-1)

	th := map[string]int64{
		"change_min": strategy.MinClose(p, cfg.ChangeMin),
		"new_high":   maxHigh(bars[len(bars)-h:]),
		"ma_short":   floorDiv(sumN, int64(n-1)) + 1,
		"ma_long":    max(0, floorDiv(int64(n)*sumM-int64(m)*sumN, int64(m-n))+1),
	}
	cmin := int64(0)
	for _, v := range th {
		if v > cmin {
			cmin = v
		}
	}
	if cmin >= strategy.MinClose(p, cfg.ChangeMax) {
		return WatchItem{}, false
	}
	avgT := 0.0
	for _, b := range bars[len(bars)-k:] {
		avgT += float64(strategy.Turnover(b))
	}
	avgT /= float64(k)
	treq := int64(math.Ceil(math.Max(float64(cfg.MinTurnover), avgT*cfg.TurnoverRatioMin)))
	return WatchItem{
		Code: sym.Code, Name: sym.Name, Market: sym.Market,
		PrevClose:    p,
		MinClose:     cmin,
		MinChangePct: float64(cmin)/float64(p) - 1,
		MinTurnover:  treq,
		MinVolume:    int64(math.Ceil(float64(treq) / float64(cmin))),
		AvgTurnover:  int64(math.Round(avgT)),
		Thresholds:   th,
	}, true
}

// FilterStatus 는 시장 필터 참고 정보: 진입가능 / 차단 / 알 수 없음(봉 부족).
func FilterStatus(cfg config.StrategyConfig, bars []data.IndexBar) string {
	if len(bars) < cfg.IndexMADays || cfg.IndexMADays <= 0 {
		return "알 수 없음"
	}
	if strategy.MarketFilter(cfg, strategy.Market{IndexBars: bars}) {
		return "진입가능"
	}
	return "차단"
}

// Result 는 Run 의 출력.
type Result struct {
	AsOf   time.Time         // 기준일 = 코스피 지수 마지막 봉 날짜. 지수가 없으면 zero
	Items  []WatchItem       // 평균 거래대금 내림차순, 같으면 코드 오름차순. 차단된 시장의 종목은 없다
	Filter map[string]string // 시장별 필터 상태
}

// Run 은 저장소의 전 종목에 Compute 를 적용한다. 기준일 이후의 봉은 무시한다.
// 기준일에 봉이 없는 종목(상장폐지·거래정지, 수집 중단 등)은 제외한다 — 그렇지 않으면 오래된 전일종가를
// 기준일 것처럼 보여주게 된다.
func Run(ctx context.Context, cfg *config.Config, store data.Store) (Result, error) {
	res := Result{Filter: map[string]string{}}
	asOf, ok, err := store.LastIndexBarDate(ctx, "kospi")
	if err != nil {
		return res, err
	}
	if !ok {
		for _, mkt := range cfg.Universe.Markets {
			res.Filter[mkt] = "알 수 없음"
		}
		return res, nil
	}
	res.AsOf = asOf
	// 거래일 ≈ 달력일 × 0.7 이라 2배 + 여유.
	days := max(cfg.Strategy.MALongDays, cfg.Strategy.TurnoverMADays, cfg.Strategy.NewHighDays)*2 + 30
	from := asOf.AddDate(0, 0, -days)
	for _, mkt := range cfg.Universe.Markets {
		ib, err := store.LoadIndexBars(ctx, mkt, from, asOf)
		if err != nil {
			return res, err
		}
		res.Filter[mkt] = FilterStatus(cfg.Strategy, ib)
	}
	syms, err := store.ListSymbols(ctx)
	if err != nil {
		return res, err
	}
	for _, s := range syms {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if res.Filter[s.Market] == "차단" {
			continue
		}
		bars, err := store.LoadBars(ctx, s.Code, from, asOf)
		if err != nil {
			return res, err
		}
		if len(bars) == 0 || !bars[len(bars)-1].Date.Equal(asOf) {
			continue
		}
		if item, ok := Compute(cfg.Strategy, s, bars); ok {
			res.Items = append(res.Items, item)
		}
	}
	sort.Slice(res.Items, func(i, j int) bool {
		if res.Items[i].AvgTurnover != res.Items[j].AvgTurnover {
			return res.Items[i].AvgTurnover > res.Items[j].AvgTurnover
		}
		return res.Items[i].Code < res.Items[j].Code
	})
	return res, nil
}

func sumClose(bars []data.Bar, n int) int64 {
	s := int64(0)
	for _, b := range bars[len(bars)-n:] {
		s += b.Close
	}
	return s
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

// floorDiv 는 음수도 내림 나눗셈.
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
