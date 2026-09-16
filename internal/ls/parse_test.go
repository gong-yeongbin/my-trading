package ls

import "testing"

func TestParseNews(t *testing.T) {
	raw := []byte(`{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"date":"20260916","time":"142800","id":"01234567","title":"삼성전자, 3분기 파운드리 수주 확대 전망","code":"005930","realkey":"","bodysize":"1234"}}`)
	ev, ok := parseMessage(raw)
	if !ok {
		t.Fatal("expected event")
	}
	n, isNews := ev.(News)
	if !isNews {
		t.Fatalf("event type = %T", ev)
	}
	if n.Title != "삼성전자, 3분기 파운드리 수주 확대 전망" || n.Date != "20260916" || n.Time != "142800" || n.ID != "01234567" || n.Code != "005930" {
		t.Errorf("news = %+v", n)
	}
}

func TestParseIndex(t *testing.T) {
	cases := []struct {
		name    string
		sign    string
		jisu    string
		drate   string
		change  string
		wantPct float64
		wantChg float64
	}{
		{"up", "2", "2712.40", "0.80", "21.50", 0.008, 21.5},
		{"down", "5", "782.15", "0.40", "3.10", -0.004, -3.1},
		{"lower-limit", "4", "700.00", "10.00", "70.00", -0.10, -70},
		{"flat", "3", "1000.00", "0.00", "0.00", 0, 0},
		{"numeric-json", "2", "", "", "", 0.008, 21.5}, // body 값이 숫자로 와도 처리
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw string
			if tc.name == "numeric-json" {
				raw = `{"header":{"tr_cd":"IJ_","tr_key":"001"},"body":{"upcode":"001","jisu":2712.40,"drate":0.80,"change":21.50,"sign":"2","time":"143210"}}`
			} else {
				raw = `{"header":{"tr_cd":"IJ_","tr_key":"301"},"body":{"upcode":"301","jisu":"` + tc.jisu + `","drate":"` + tc.drate + `","change":"` + tc.change + `","sign":"` + tc.sign + `","time":"143210"}}`
			}
			ev, ok := parseMessage([]byte(raw))
			if !ok {
				t.Fatal("expected event")
			}
			ix, isIndex := ev.(Index)
			if !isIndex {
				t.Fatalf("event type = %T", ev)
			}
			if tc.name != "numeric-json" && ix.Code != "301" {
				t.Errorf("code = %q", ix.Code)
			}
			if !near(ix.ChangePct, tc.wantPct) || !near(ix.Change, tc.wantChg) {
				t.Errorf("pct = %v want %v, change = %v want %v", ix.ChangePct, tc.wantPct, ix.Change, tc.wantChg)
			}
			if ix.Time != "143210" {
				t.Errorf("time = %q", ix.Time)
			}
		})
	}
}

func TestParseSubscribeResponse(t *testing.T) {
	ev, ok := parseMessage([]byte(`{"header":{"tr_cd":"NWS","rsp_cd":"00000","rsp_msg":"정상처리"}}`))
	if ev != nil || ok {
		t.Errorf("success ack: got (%+v, %v), want (nil, false)", ev, ok)
	}

	ev, ok = parseMessage([]byte(`{"header":{"tr_cd":"IJ_","rsp_cd":"E1234","rsp_msg":"권한 없음"}}`))
	if !ok {
		t.Fatal("expected SubscribeError event")
	}
	se, isSE := ev.(SubscribeError)
	if !isSE || se.TrCd != "IJ_" || se.Code != "E1234" || se.Msg != "권한 없음" {
		t.Errorf("subscribe error = %+v", ev)
	}
}

func TestParseIndexCodeFallsBackToHeaderTrKey(t *testing.T) {
	raw := `{"header":{"tr_cd":"IJ_","tr_key":"001"},"body":{"jisu":"2712.40","drate":"0.80","change":"21.50","sign":"2","time":"143210"}}`
	ev, ok := parseMessage([]byte(raw))
	if !ok {
		t.Fatal("expected event")
	}
	ix, isIndex := ev.(Index)
	if !isIndex || ix.Code != "001" {
		t.Errorf("index = %+v", ev)
	}
}

func TestParseNewsDropsBlankAndStripsNewlines(t *testing.T) {
	ev, ok := parseMessage([]byte(`{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"title":"  "}}`))
	if ev != nil || ok {
		t.Errorf("blank title: got (%+v, %v), want (nil, false)", ev, ok)
	}

	ev, ok = parseMessage([]byte(`{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"title":"줄1\n줄2"}}`))
	if !ok {
		t.Fatal("expected event")
	}
	n, isNews := ev.(News)
	if !isNews || n.Title != "줄1 줄2" {
		t.Errorf("news = %+v", ev)
	}
}

func TestParseIgnoresOthers(t *testing.T) {
	for _, raw := range []string{
		`{"header":{"tr_cd":"NWS","rsp_cd":"00000","rsp_msg":"정상처리"}}`,                              // 구독 응답 (body 없음)
		`{"header":{"tr_cd":"IJ_","tr_key":"001"},"body":{"upcode":"001","jisu":"abc","sign":"2"}}`, // 숫자 아님
		`not json`,
	} {
		if ev, ok := parseMessage([]byte(raw)); ok {
			t.Errorf("expected no event for %s, got %+v", raw, ev)
		}
	}
}

func TestParseUnknownTRAsRaw(t *testing.T) {
	ev, ok := parseMessage([]byte(`{"header":{"tr_cd":"JIF","tr_key":"1"},"body":{"jangubun":"1","jstatus":"21"}}`))
	if !ok {
		t.Fatal("expected Raw event")
	}
	r, isRaw := ev.(Raw)
	if !isRaw || r.TrCd != "JIF" || r.TrKey != "1" || r.Body["jstatus"] != "21" {
		t.Errorf("raw = %+v", ev)
	}
}

func near(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
