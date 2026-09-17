package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

// businessDays 는 [from, to] 의 평일을 오름차순으로 돌려준다.
func businessDays(from, to time.Time) []time.Time {
	var out []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			out = append(out, d)
		}
	}
	return out
}

// serveItemChart 는 요청 기간의 평일 봉을 최신순으로 최대 100건 돌려준다 (실서버와 같은 제한).
// blank 에 든 날짜는 가격이 빈 문자열인 행으로 돌려준다.
func serveItemChart(t *testing.T, f *fakeKIS, calls *[][2]string, blank map[string]bool) {
	t.Helper()
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("tr_id") != "FHKST03010100" || q.Get("FID_COND_MRKT_DIV_CODE") != "J" || q.Get("FID_PERIOD_DIV_CODE") != "D" ||
			q.Get("FID_ORG_ADJ_PRC") != "0" || q.Get("FID_INPUT_ISCD") != "005930" {
			http.Error(w, "bad params: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		d1, d2 := q.Get("FID_INPUT_DATE_1"), q.Get("FID_INPUT_DATE_2")
		*calls = append(*calls, [2]string{d1, d2})
		from, _ := time.ParseInLocation("20060102", d1, data.KST)
		to, _ := time.ParseInLocation("20060102", d2, data.KST)
		days := businessDays(from, to)
		var rows []map[string]string
		for i := len(days) - 1; i >= 0 && len(rows) < 100; i-- {
			ds := days[i].Format("20060102")
			row := map[string]string{"stck_bsop_date": ds, "stck_oprc": "100", "stck_hgpr": "110", "stck_lwpr": "90",
				"stck_clpr": fmt.Sprint(1000 + days[i].YearDay()), "acml_vol": "5000", "acml_tr_pbmn": "1", "flng_cls_code": "00", "prtt_rate": "0.00", "mod_yn": "N"}
			if blank[ds] {
				row["stck_oprc"], row["stck_hgpr"], row["stck_lwpr"], row["stck_clpr"], row["acml_vol"] = "", "", "", "", ""
			}
			rows = append(rows, row)
		}
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output1": map[string]string{}, "output2": rows})
	})
}

func TestDailyBarsChunksAndSorts(t *testing.T) {
	f := newFakeKIS(t)
	var calls [][2]string
	serveItemChart(t, f, &calls, map[string]bool{"20240215": true})
	from, to := data.Date(2024, 1, 2), data.Date(2024, 12, 31)
	bars, err := f.client.DailyBars(context.Background(), "005930", from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := businessDays(from, to)
	if len(bars) != len(want)-1 {
		t.Fatalf("got %d bars, want %d (one blank row skipped)", len(bars), len(want)-1)
	}
	for i := 1; i < len(bars); i++ {
		if !bars[i].Date.After(bars[i-1].Date) {
			t.Fatalf("not strictly ascending at %d: %v %v", i, bars[i-1].Date, bars[i].Date)
		}
	}
	if !bars[0].Date.Equal(from) || !bars[len(bars)-1].Date.Equal(data.Date(2024, 12, 31)) {
		t.Errorf("range: first=%v last=%v", bars[0].Date, bars[len(bars)-1].Date)
	}
	if bars[0].Close != 1002 || bars[0].Volume != 5000 || bars[0].High != 110 {
		t.Errorf("first bar = %+v", bars[0])
	}
	if len(calls) < 3 || calls[0][1] != "20241231" || calls[len(calls)-1][0] != "20240102" {
		t.Errorf("calls = %v", calls)
	}
	for _, c := range calls {
		a, _ := time.Parse("20060102", c[0])
		b, _ := time.Parse("20060102", c[1])
		if b.Sub(a) > 139*24*time.Hour {
			t.Errorf("chunk too wide: %v", c)
		}
	}
}

func TestDailyBarsStopsOnEmptyResponse(t *testing.T) {
	f := newFakeKIS(t)
	var calls [][2]string
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		calls = append(calls, [2]string{q.Get("FID_INPUT_DATE_1"), q.Get("FID_INPUT_DATE_2")})
		if q.Get("FID_INPUT_DATE_2") < "20240902" { // 첫 청크(~20240902)는 데이터 1건, 그 전날부터 시작하는 둘째 청크는 빈 응답
			json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": []map[string]string{
			{"stck_bsop_date": "20240902", "stck_oprc": "1", "stck_hgpr": "2", "stck_lwpr": "1", "stck_clpr": "2", "acml_vol": "3"},
		}})
	})
	bars, err := f.client.DailyBars(context.Background(), "005930", data.Date(2020, 1, 1), data.Date(2024, 9, 2))
	if err != nil || len(bars) != 1 {
		t.Fatalf("bars=%v err=%v", bars, err)
	}
	if len(calls) != 2 {
		t.Errorf("expected stop after first empty chunk, calls=%v", calls)
	}
}

func TestDailyBarsFromAfterTo(t *testing.T) {
	f := newFakeKIS(t)
	bars, err := f.client.DailyBars(context.Background(), "005930", data.Date(2024, 9, 3), data.Date(2024, 9, 2))
	if err != nil || len(bars) != 0 {
		t.Errorf("expected no calls and no bars: %v %v", bars, err)
	}
}

func TestIndexBars(t *testing.T) {
	f := newFakeKIS(t)
	var gotCode string
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("tr_id") != "FHKUP03500100" || q.Get("FID_COND_MRKT_DIV_CODE") != "U" || q.Get("FID_PERIOD_DIV_CODE") != "D" {
			http.Error(w, "bad params: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		gotCode = q.Get("FID_INPUT_ISCD")
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": []map[string]string{
			{"stck_bsop_date": "20240903", "bstp_nmix_prpr": "2664.63", "bstp_nmix_oprc": "2680.10", "bstp_nmix_hgpr": "2690.55", "bstp_nmix_lwpr": "2660.01", "acml_vol": "1"},
			{"stck_bsop_date": "20240902", "bstp_nmix_prpr": "2681.00", "bstp_nmix_oprc": "2670.00", "bstp_nmix_hgpr": "2685.00", "bstp_nmix_lwpr": "2665.00", "acml_vol": "1"},
		}})
	})
	bars, err := f.client.IndexBars(context.Background(), "kosdaq", data.Date(2024, 9, 1), data.Date(2024, 9, 3))
	if err != nil {
		t.Fatal(err)
	}
	if gotCode != "1001" {
		t.Errorf("kosdaq index code = %q, want 1001", gotCode)
	}
	if len(bars) != 2 || !bars[0].Date.Equal(data.Date(2024, 9, 2)) || bars[1].Close != 2664.63 || bars[1].Open != 2680.10 {
		t.Errorf("bars = %+v", bars)
	}
	if _, err := f.client.IndexBars(context.Background(), "nyse", data.Date(2024, 9, 1), data.Date(2024, 9, 3)); err == nil {
		t.Error("unknown market should error before calling")
	}
	f.client.IndexBars(context.Background(), "kospi", data.Date(2024, 9, 1), data.Date(2024, 9, 3))
	if gotCode != "0001" {
		t.Errorf("kospi index code = %q, want 0001", gotCode)
	}
}

// 실서버 지수 API 는 한 번에 50건만 돌려준다. 청크가 그보다 넓어도 빠짐없이 이어받아야 한다.
func TestIndexBarsFollowsOldestReturnedRow(t *testing.T) {
	f := newFakeKIS(t)
	var calls [][2]string
	f.handle("/uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		d1, d2 := q.Get("FID_INPUT_DATE_1"), q.Get("FID_INPUT_DATE_2")
		calls = append(calls, [2]string{d1, d2})
		from, _ := time.ParseInLocation("20060102", d1, data.KST)
		to, _ := time.ParseInLocation("20060102", d2, data.KST)
		days := businessDays(from, to)
		var rows []map[string]string
		for i := len(days) - 1; i >= 0 && len(rows) < 50; i-- {
			rows = append(rows, map[string]string{"stck_bsop_date": days[i].Format("20060102"), "bstp_nmix_prpr": "1", "bstp_nmix_oprc": "1", "bstp_nmix_hgpr": "1", "bstp_nmix_lwpr": "1", "acml_vol": "1"})
		}
		json.NewEncoder(w).Encode(map[string]any{"rt_cd": "0", "msg_cd": "MCA00000", "msg1": "ok", "output2": rows})
	})
	from, to := data.Date(2025, 9, 1), data.Date(2026, 9, 17)
	bars, err := f.client.IndexBars(context.Background(), "kospi", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if want := businessDays(from, to); len(bars) != len(want) || !bars[0].Date.Equal(from) {
		t.Errorf("got %d bars from %v, want %d from %v (calls=%v)", len(bars), bars[0].Date, len(want), from, calls)
	}
}
