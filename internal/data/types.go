// Package data 는 시세 타입과 로컬 저장소를 정의한다.
package data

import "time"

// KST 는 모든 날짜의 기준 시간대다. 봉의 Date 는 KST 자정이다.
var KST = time.FixedZone("KST", 9*3600)

type Symbol struct {
	Code   string // 6자리 단축코드
	Name   string
	Market string // kospi | kosdaq
}

type Bar struct {
	Date   time.Time
	Open   int64
	High   int64
	Low    int64
	Close  int64
	Volume int64
}

type IndexBar struct {
	Date  time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
}

const dateLayout = "2006-01-02"

func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, KST)
}

func FormatDate(t time.Time) string {
	return t.In(KST).Format(dateLayout)
}

// Date 는 y-m-d 를 KST 자정으로 만든다. 테스트와 CLI 에서 쓴다.
func Date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, KST)
}
