package ls

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type wsMessage struct {
	Header struct {
		TrCd   string `json:"tr_cd"`
		TrKey  string `json:"tr_key"`
		RspCd  string `json:"rsp_cd"`
		RspMsg string `json:"rsp_msg"`
	} `json:"header"`
	Body map[string]any `json:"body"`
}

// parseMessage 는 수신 JSON 을 Event 로 바꾼다. 구독 성공 응답, 모르는 TR, 깨진 숫자는 (nil, false).
// 구독 거부 응답은 SubscribeError.
func parseMessage(data []byte) (Event, bool) {
	var m wsMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false
	}
	if m.Header.RspCd != "" {
		if m.Header.RspCd == "00000" {
			return nil, false
		}
		return SubscribeError{TrCd: m.Header.TrCd, Code: m.Header.RspCd, Msg: m.Header.RspMsg}, true
	}
	if m.Body == nil {
		return nil, false
	}
	switch m.Header.TrCd {
	case "NWS":
		title := strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(field(m.Body, "title")))
		if title == "" {
			return nil, false
		}
		return News{
			Date:  field(m.Body, "date"),
			Time:  field(m.Body, "time"),
			ID:    field(m.Body, "id"),
			Title: title,
			Code:  field(m.Body, "code"),
		}, true
	case "IJ_":
		value, err1 := strconv.ParseFloat(field(m.Body, "jisu"), 64)
		change, err2 := strconv.ParseFloat(field(m.Body, "change"), 64)
		pct, err3 := strconv.ParseFloat(field(m.Body, "drate"), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, false
		}
		if s := field(m.Body, "sign"); s == "4" || s == "5" {
			change, pct = -abs(change), -abs(pct)
		} else {
			change, pct = abs(change), abs(pct)
		}
		code := field(m.Body, "upcode")
		if code == "" {
			code = m.Header.TrKey
		}
		return Index{
			Code:      code,
			Value:     value,
			Change:    change,
			ChangePct: pct / 100,
			Time:      field(m.Body, "time"),
		}, true
	default:
		if m.Header.TrCd == "" {
			return nil, false
		}
		return Raw{TrCd: m.Header.TrCd, TrKey: m.Header.TrKey, Body: m.Body}, true
	}
}

// field 는 body 값을 문자열로 꺼낸다. LS 는 문자열로 보내지만 숫자로 와도 처리한다.
func field(body map[string]any, key string) string {
	switch v := body[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
