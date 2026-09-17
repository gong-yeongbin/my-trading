package kis

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const balancePath = "/uapi/domestic-stock/v1/trading/inquire-balance"

// Account 는 한투 계좌번호 "12345678-01" 을 나눈 것.
type Account struct {
	CANO    string // 앞 8자리
	Product string // 뒤 2자리 (계좌상품코드)
}

var accountRe = regexp.MustCompile(`^(\d{8})-(\d{2})$`)

func ParseAccount(s string) (Account, error) {
	m := accountRe.FindStringSubmatch(s)
	if m == nil {
		return Account{}, fmt.Errorf("kis: 계좌번호는 12345678-01 형식이어야 합니다: %q", s)
	}
	return Account{CANO: m[1], Product: m[2]}, nil
}

// Position 은 보유 종목 한 줄. 금액은 원 단위 정수(반올림), PnLPct 는 0.024 = +2.4%.
type Position struct {
	Code, Name string
	Qty        int64
	AvgPrice   int64
	Price      int64
	PnL        int64
	PnLPct     float64
}

// Balance 는 계좌 잔고. Total 평가금액, Cash 예수금, PnL 평가손익 합계.
type Balance struct {
	Positions []Position
	Total     int64
	Cash      int64
	PnL       int64
}

type balanceResp struct {
	CtxFK   string `json:"ctx_area_fk100"`
	CtxNK   string `json:"ctx_area_nk100"`
	Output1 []struct {
		Code     string `json:"pdno"`
		Name     string `json:"prdt_name"`
		Qty      string `json:"hldg_qty"`
		AvgPrice string `json:"pchs_avg_pric"`
		Price    string `json:"prpr"`
		PnL      string `json:"evlu_pfls_amt"`
		PnLPct   string `json:"evlu_pfls_rt"`
	} `json:"output1"`
	Output2 []struct {
		Total string `json:"tot_evlu_amt"`
		Cash  string `json:"dnca_tot_amt"`
		PnL   string `json:"evlu_pfls_smtl_amt"`
	} `json:"output2"`
}

// Balance 는 잔고를 조회한다. demo 면 모의투자 tr_id. 다음 페이지가 있으면 이어받는다 (최대 20페이지).
func (c *Client) Balance(ctx context.Context, acct Account, demo bool) (Balance, error) {
	trID := "TTTC8434R"
	if demo {
		trID = "VTTC8434R"
	}
	var out Balance
	fk, nk, cont := "", "", ""
	for page := 0; page < 20; page++ {
		params := url.Values{
			"CANO": {acct.CANO}, "ACNT_PRDT_CD": {acct.Product},
			"AFHR_FLPR_YN": {"N"}, "OFL_YN": {""}, "INQR_DVSN": {"02"}, "UNPR_DVSN": {"01"},
			"FUND_STTL_ICLD_YN": {"N"}, "FNCG_AMT_AUTO_RDPT_YN": {"N"}, "PRCS_DVSN": {"01"},
			"CTX_AREA_FK100": {fk}, "CTX_AREA_NK100": {nk},
		}
		var resp balanceResp
		next, err := c.getCont(ctx, balancePath, trID, cont, params, &resp)
		if err != nil {
			return Balance{}, err
		}
		for _, r := range resp.Output1 {
			qty := num(r.Qty)
			if qty <= 0 {
				continue
			}
			out.Positions = append(out.Positions, Position{
				Code: r.Code, Name: r.Name, Qty: qty,
				AvgPrice: num(r.AvgPrice), Price: num(r.Price), PnL: num(r.PnL),
				PnLPct: fnum(r.PnLPct) / 100,
			})
		}
		if len(resp.Output2) > 0 {
			out.Total, out.Cash, out.PnL = num(resp.Output2[0].Total), num(resp.Output2[0].Cash), num(resp.Output2[0].PnL)
		}
		if next != "F" && next != "M" {
			break
		}
		fk, nk, cont = resp.CtxFK, resp.CtxNK, "N"
	}
	return out, nil
}

func fnum(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func num(s string) int64 { return int64(math.Round(fnum(s))) }
