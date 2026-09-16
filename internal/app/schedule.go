package app

import (
	"fmt"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/data"
)

// NextRun 은 now 이후 처음 오는 dailyAt(HH:MM, KST) 시각. now 가 정확히 그 시각이면 다음 날.
func NextRun(now time.Time, dailyAt string) (time.Time, error) {
	hm, err := time.Parse("15:04", dailyAt)
	if err != nil || len(dailyAt) != 5 {
		return time.Time{}, fmt.Errorf("daily_at must be HH:MM: %q", dailyAt)
	}
	n := now.In(data.KST)
	next := time.Date(n.Year(), n.Month(), n.Day(), hm.Hour(), hm.Minute(), 0, 0, data.KST)
	if !next.After(n) {
		next = next.AddDate(0, 0, 1)
	}
	return next, nil
}

// Stale 은 지수 마지막 봉이 "오늘 이전의 마지막 평일" 보다 오래됐는지. 봉이 없으면 true. 공휴일은 모른다.
func Stale(lastIndex time.Time, has bool, now time.Time) bool {
	if !has {
		return true
	}
	d := now.In(data.KST)
	prev := data.Date(d.Year(), d.Month(), d.Day()).AddDate(0, 0, -1)
	for prev.Weekday() == time.Saturday || prev.Weekday() == time.Sunday {
		prev = prev.AddDate(0, 0, -1)
	}
	return lastIndex.In(data.KST).Before(prev)
}
