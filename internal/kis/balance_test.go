package kis

import (
	"context"
	"net/http"
	"testing"
)

func TestParseAccount(t *testing.T) {
	a, err := ParseAccount("12345678-01")
	if err != nil || a.CANO != "12345678" || a.Product != "01" {
		t.Errorf("parse = %+v %v", a, err)
	}
	for _, bad := range []string{"", "1234567801", "12345678-1", "1234567-01", "abcdefgh-01"} {
		if _, err := ParseAccount(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

const balancePage1 = `{"rt_cd":"0","msg_cd":"KIOK0510","msg1":"조회되었습니다",
 "ctx_area_fk100":"FK1","ctx_area_nk100":"NK1",
 "output1":[
  {"pdno":"005930","prdt_name":"삼성전자","hldg_qty":"58","pchs_avg_pric":"71200.0000","prpr":"72900","evlu_pfls_amt":"98600","evlu_pfls_rt":"2.40"},
  {"pdno":"000660","prdt_name":"SK하이닉스","hldg_qty":"0","pchs_avg_pric":"0.0000","prpr":"180000","evlu_pfls_amt":"0","evlu_pfls_rt":"0.00"}
 ],
 "output2":[{"tot_evlu_amt":"12480000","dnca_tot_amt":"7520000","evlu_pfls_smtl_amt":"312000"}]}`

const balancePage2 = `{"rt_cd":"0","msg_cd":"KIOK0510","msg1":"조회되었습니다",
 "ctx_area_fk100":"","ctx_area_nk100":"",
 "output1":[{"pdno":"247540","prdt_name":"에코프로비엠","hldg_qty":"40","pchs_avg_pric":"98500.5000","prpr":"96100","evlu_pfls_amt":"-96020","evlu_pfls_rt":"-2.44"}],
 "output2":[{"tot_evlu_amt":"12480000","dnca_tot_amt":"7520000","evlu_pfls_smtl_amt":"312000"}]}`

func TestBalancePagesAndParses(t *testing.T) {
	f := newFakeKIS(t)
	var calls []*http.Request
	f.handle("/uapi/domestic-stock/v1/trading/inquire-balance", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r)
		switch r.Header.Get("tr_cont") {
		case "":
			w.Header().Set("tr_cont", "M")
			w.Write([]byte(balancePage1))
		case "N":
			w.Header().Set("tr_cont", "D")
			w.Write([]byte(balancePage2))
		default:
			t.Errorf("unexpected tr_cont %q", r.Header.Get("tr_cont"))
		}
	})
	c := f.client
	b, err := c.Balance(context.Background(), Account{CANO: "12345678", Product: "01"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	q := calls[0].URL.Query()
	for k, want := range map[string]string{"CANO": "12345678", "ACNT_PRDT_CD": "01", "AFHR_FLPR_YN": "N", "INQR_DVSN": "02", "UNPR_DVSN": "01", "FUND_STTL_ICLD_YN": "N", "FNCG_AMT_AUTO_RDPT_YN": "N", "PRCS_DVSN": "01", "CTX_AREA_FK100": "", "CTX_AREA_NK100": ""} {
		if got := q.Get(k); got != want {
			t.Errorf("param %s = %q, want %q", k, got, want)
		}
	}
	if calls[0].Header.Get("tr_id") != "TTTC8434R" {
		t.Errorf("tr_id = %q", calls[0].Header.Get("tr_id"))
	}
	q2 := calls[1].URL.Query()
	if q2.Get("CTX_AREA_FK100") != "FK1" || q2.Get("CTX_AREA_NK100") != "NK1" {
		t.Errorf("continuation params = %v", q2)
	}
	if len(b.Positions) != 2 { // 수량 0 제외
		t.Fatalf("positions = %+v", b.Positions)
	}
	p := b.Positions[0]
	if p.Code != "005930" || p.Name != "삼성전자" || p.Qty != 58 || p.AvgPrice != 71200 || p.Price != 72900 || p.PnL != 98600 || p.PnLPct != 0.024 {
		t.Errorf("position = %+v", p)
	}
	if b.Positions[1].AvgPrice != 98501 || b.Positions[1].PnL != -96020 { // 98500.5 반올림
		t.Errorf("position 2 = %+v", b.Positions[1])
	}
	if b.Total != 12480000 || b.Cash != 7520000 || b.PnL != 312000 {
		t.Errorf("summary = %+v", b)
	}
}

func TestBalanceDemoTrID(t *testing.T) {
	f := newFakeKIS(t)
	var trID string
	f.handle("/uapi/domestic-stock/v1/trading/inquire-balance", func(w http.ResponseWriter, r *http.Request) {
		trID = r.Header.Get("tr_id")
		w.Write([]byte(balancePage2))
	})
	if _, err := f.client.Balance(context.Background(), Account{CANO: "12345678", Product: "01"}, true); err != nil {
		t.Fatal(err)
	}
	if trID != "VTTC8434R" {
		t.Errorf("demo tr_id = %q", trID)
	}
}

func TestBalanceBusinessError(t *testing.T) {
	f := newFakeKIS(t)
	f.handle("/uapi/domestic-stock/v1/trading/inquire-balance", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기타오류"}`))
	})
	if _, err := f.client.Balance(context.Background(), Account{CANO: "12345678", Product: "01"}, false); err == nil {
		t.Error("expected error")
	}
}
