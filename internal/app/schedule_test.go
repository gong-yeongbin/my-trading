package app

import (
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

func kst(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, data.KST)
}

func TestNextRun(t *testing.T) {
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{kst(2026, 9, 16, 3, 59), kst(2026, 9, 16, 4, 0)},  // 오늘 04:00 전 → 오늘
		{kst(2026, 9, 16, 4, 0), kst(2026, 9, 17, 4, 0)},   // 정각이면 내일
		{kst(2026, 9, 16, 22, 30), kst(2026, 9, 17, 4, 0)}, // 밤 → 내일
		{kst(2026, 12, 31, 23, 0), kst(2027, 1, 1, 4, 0)},  // 연말 → 새해
	}
	for _, tc := range cases {
		got, err := NextRun(tc.now, "04:00")
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("NextRun(%v) = %v, %v; want %v", tc.now, got, err, tc.want)
		}
	}
	// 다른 시간대로 들어와도 KST 기준
	utc := time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC) // KST 16 05:00
	got, _ := NextRun(utc, "04:00")
	if !got.Equal(kst(2026, 9, 17, 4, 0)) {
		t.Errorf("UTC input: %v", got)
	}
	if _, err := NextRun(kst(2026, 9, 16, 0, 0), "4:00"); err == nil {
		t.Error("bad format should error")
	}
}

func TestStale(t *testing.T) {
	cases := []struct {
		name string
		last time.Time
		has  bool
		now  time.Time
		want bool
	}{
		{"no data", time.Time{}, false, kst(2026, 9, 16, 9, 0), true},
		{"tue morning, has mon", data.Date(2026, 9, 14), true, kst(2026, 9, 15, 9, 0), false},
		{"tue morning, has fri", data.Date(2026, 9, 11), true, kst(2026, 9, 15, 9, 0), true},
		{"mon morning, has fri", data.Date(2026, 9, 11), true, kst(2026, 9, 14, 9, 0), false},
		{"sat, has fri", data.Date(2026, 9, 18), true, kst(2026, 9, 19, 12, 0), false},
		{"sun, has thu", data.Date(2026, 9, 17), true, kst(2026, 9, 20, 12, 0), true},
		{"has today already", data.Date(2026, 9, 16), true, kst(2026, 9, 16, 23, 0), false},
	}
	for _, tc := range cases {
		if got := Stale(tc.last, tc.has, tc.now); got != tc.want {
			t.Errorf("%s: Stale = %v, want %v", tc.name, got, tc.want)
		}
	}
}
