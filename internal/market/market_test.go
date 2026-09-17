package market

import (
	"testing"
	"time"
)

func at(day int, hh, mm int) time.Time {
	// 2026-09-14 월요일. day 는 그 주의 날짜.
	return time.Date(2026, 9, day, hh, mm, 0, 0, KST)
}

func TestStatusByClock(t *testing.T) {
	cases := []struct {
		t    time.Time
		want string
	}{
		{at(14, 8, 59), "장전"},
		{at(14, 9, 0), "장중"},
		{at(14, 15, 19), "장중"},
		{at(14, 15, 20), "동시호가"},
		{at(14, 15, 29), "동시호가"},
		{at(14, 15, 30), "장마감"},
		{at(14, 15, 59), "장마감"},
		{at(14, 16, 0), "시간외"},
		{at(14, 17, 59), "시간외"},
		{at(14, 18, 0), "장마감"},
		{at(14, 23, 30), "장마감"},
		{at(19, 10, 0), "휴장"}, // 토
		{at(20, 10, 0), "휴장"}, // 일
	}
	for _, tc := range cases {
		if got := At(tc.t).String(); got != tc.want {
			t.Errorf("At(%v) = %q, want %q", tc.t.Format("Mon 15:04"), got, tc.want)
		}
	}
	// 다른 시간대로 들어와도 KST 로 판정한다
	utc := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) // KST 10:00
	if got := At(utc).String(); got != "장중" {
		t.Errorf("UTC input: %q", got)
	}
}

func TestFromJIF(t *testing.T) {
	cases := map[string]string{"11": "장전", "21": "장중", "31": "동시호가", "41": "장마감", "51": "시간외", "52": "장마감", "61": "시간외", "62": "장마감"}
	for code, want := range cases {
		got, ok := FromJIF(code)
		if !ok || got.String() != want {
			t.Errorf("FromJIF(%s) = %q,%v want %q", code, got, ok, want)
		}
	}
	for _, code := range []string{"", "22", "99", "abc"} {
		if got, ok := FromJIF(code); ok {
			t.Errorf("FromJIF(%q) should be unknown, got %q", code, got)
		}
	}
}

func TestTrading(t *testing.T) {
	for s, want := range map[Status]bool{Closed: false, PreOpen: false, Open: true, ClosingAuction: true, AfterClose: false, AfterHours: false} {
		if s.Trading() != want {
			t.Errorf("%s.Trading() = %v", s, s.Trading())
		}
	}
	if Status(99).String() != "알 수 없음" {
		t.Error("unknown status string")
	}
}
