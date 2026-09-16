package ls

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type wsMessage struct {
	Header struct {
		TrCd  string `json:"tr_cd"`
		TrKey string `json:"tr_key"`
	} `json:"header"`
	Body map[string]any `json:"body"`
}

// parseMessage 는 수신 JSON 을 Event 로 바꾼다. 구독 응답, 모르는 TR, 깨진 숫자는 (nil, false).
func parseMessage(data []byte) (Event, bool) {
	var m wsMessage
	if err := json.Unmarshal(data, &m); err != nil || m.Body == nil {
		return nil, false
	}
	switch m.Header.TrCd {
	case "NWS":
		return News{
			Date:  field(m.Body, "date"),
			Time:  field(m.Body, "time"),
			ID:    field(m.Body, "id"),
			Title: field(m.Body, "title"),
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
		return Index{
			Code:      field(m.Body, "upcode"),
			Value:     value,
			Change:    change,
			ChangePct: pct / 100,
			Time:      field(m.Body, "time"),
		}, true
	}
	return nil, false
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
