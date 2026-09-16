package kis

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

const (
	itemChartPath = "/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice"
	itemChartTrID = "FHKST03010100"
	// 호출당 최대 100건. 달력일 140일 ≈ 영업일 96일이라 한 번에 다 들어온다.
	chunkCalendarDays = 140
	apiDateLayout     = "20060102"
)

type itemRow struct {
	Date   string `json:"stck_bsop_date"`
	Open   string `json:"stck_oprc"`
	High   string `json:"stck_hgpr"`
	Low    string `json:"stck_lwpr"`
	Close  string `json:"stck_clpr"`
	Volume string `json:"acml_vol"`
}

// DailyBars 는 [from, to] 의 수정주가 일봉을 오름차순으로 돌려준다.
// 기간을 종료일에서 거꾸로 140일씩 잘라 호출하고, 빈 응답을 받으면 멈춘다.
func (c *Client) DailyBars(ctx context.Context, code string, from, to time.Time) ([]data.Bar, error) {
	byDate := map[string]data.Bar{}
	err := c.walkChunks(from, to, func(start, end time.Time) (int, error) {
		var resp struct {
			Output2 []itemRow `json:"output2"`
		}
		params := url.Values{
			"FID_COND_MRKT_DIV_CODE": {"J"},
			"FID_INPUT_ISCD":         {code},
			"FID_INPUT_DATE_1":       {start.In(data.KST).Format(apiDateLayout)},
			"FID_INPUT_DATE_2":       {end.In(data.KST).Format(apiDateLayout)},
			"FID_PERIOD_DIV_CODE":    {"D"},
			"FID_ORG_ADJ_PRC":        {"0"},
		}
		if err := c.get(ctx, itemChartPath, itemChartTrID, params, &resp); err != nil {
			return 0, err
		}
		for _, r := range resp.Output2 {
			if r.Close == "" || r.Open == "" {
				continue // 거래정지 등으로 가격이 비어 있는 행
			}
			b, err := parseItemRow(r)
			if err != nil {
				return 0, fmt.Errorf("kis: %s %s: %w", code, r.Date, err)
			}
			byDate[r.Date] = b
		}
		return len(resp.Output2), nil
	})
	if err != nil {
		return nil, err
	}
	return sortBars(byDate), nil
}

// walkChunks 는 to 에서 from 쪽으로 chunkCalendarDays 단위로 call 을 호출한다. call 이 0건을 돌려주면 멈춘다.
func (c *Client) walkChunks(from, to time.Time, call func(start, end time.Time) (int, error)) error {
	end := to
	for !end.Before(from) {
		start := end.AddDate(0, 0, -(chunkCalendarDays - 1))
		if start.Before(from) {
			start = from
		}
		n, err := call(start, end)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		end = start.AddDate(0, 0, -1)
	}
	return nil
}

func parseItemRow(r itemRow) (data.Bar, error) {
	d, err := time.ParseInLocation(apiDateLayout, r.Date, data.KST)
	if err != nil {
		return data.Bar{}, err
	}
	vals := [5]int64{}
	for i, s := range []string{r.Open, r.High, r.Low, r.Close, r.Volume} {
		if vals[i], err = strconv.ParseInt(s, 10, 64); err != nil {
			return data.Bar{}, fmt.Errorf("field %d %q: %w", i, s, err)
		}
	}
	return data.Bar{Date: d, Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3], Volume: vals[4]}, nil
}

func sortBars(byDate map[string]data.Bar) []data.Bar {
	out := make([]data.Bar, 0, len(byDate))
	for _, b := range byDate {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

const (
	indexChartPath = "/uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice"
	indexChartTrID = "FHKUP03500100"
)

var indexCodes = map[string]string{"kospi": "0001", "kosdaq": "1001"}

type indexRow struct {
	Date  string `json:"stck_bsop_date"`
	Open  string `json:"bstp_nmix_oprc"`
	High  string `json:"bstp_nmix_hgpr"`
	Low   string `json:"bstp_nmix_lwpr"`
	Close string `json:"bstp_nmix_prpr"`
}

// IndexBars 는 시장 종합지수(코스피 0001, 코스닥 1001)의 일봉을 오름차순으로 돌려준다.
func (c *Client) IndexBars(ctx context.Context, market string, from, to time.Time) ([]data.IndexBar, error) {
	code, ok := indexCodes[market]
	if !ok {
		return nil, fmt.Errorf("kis: unknown market %q", market)
	}
	byDate := map[string]data.IndexBar{}
	err := c.walkChunks(from, to, func(start, end time.Time) (int, error) {
		var resp struct {
			Output2 []indexRow `json:"output2"`
		}
		params := url.Values{
			"FID_COND_MRKT_DIV_CODE": {"U"},
			"FID_INPUT_ISCD":         {code},
			"FID_INPUT_DATE_1":       {start.In(data.KST).Format(apiDateLayout)},
			"FID_INPUT_DATE_2":       {end.In(data.KST).Format(apiDateLayout)},
			"FID_PERIOD_DIV_CODE":    {"D"},
		}
		if err := c.get(ctx, indexChartPath, indexChartTrID, params, &resp); err != nil {
			return 0, err
		}
		for _, r := range resp.Output2 {
			if r.Close == "" {
				continue
			}
			b, err := parseIndexRow(r)
			if err != nil {
				return 0, fmt.Errorf("kis: index %s %s: %w", market, r.Date, err)
			}
			byDate[r.Date] = b
		}
		return len(resp.Output2), nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]data.IndexBar, 0, len(byDate))
	for _, b := range byDate {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

func parseIndexRow(r indexRow) (data.IndexBar, error) {
	d, err := time.ParseInLocation(apiDateLayout, r.Date, data.KST)
	if err != nil {
		return data.IndexBar{}, err
	}
	vals := [4]float64{}
	for i, s := range []string{r.Open, r.High, r.Low, r.Close} {
		if vals[i], err = strconv.ParseFloat(s, 64); err != nil {
			return data.IndexBar{}, fmt.Errorf("field %d %q: %w", i, s, err)
		}
	}
	return data.IndexBar{Date: d, Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3]}, nil
}
