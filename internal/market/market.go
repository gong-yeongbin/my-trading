// Package market 은 국내 주식 장 상태를 판정한다. 시계 기준 판정과 LS 장운영정보(JIF) 코드표를 제공한다.
// 공휴일은 반영하지 않는다 (JIF 가 오면 그 값이 우선).
package market

import "time"

// KST 는 tzdata 없이도 동작하도록 고정 오프셋으로 둔다.
var KST = time.FixedZone("KST", 9*60*60)

// Status 는 t 시각의 장 상태 문구: 휴장 / 장전 / 장중 / 동시호가 / 장마감 / 시간외.
func Status(t time.Time) string {
	t = t.In(KST)
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return "휴장"
	}
	hm := t.Hour()*60 + t.Minute()
	switch {
	case hm < 9*60:
		return "장전"
	case hm < 15*60+20:
		return "장중"
	case hm < 15*60+30:
		return "동시호가"
	case hm < 16*60:
		return "장마감"
	case hm < 18*60:
		return "시간외"
	default:
		return "장마감"
	}
}

// jifStatus 는 LS 장운영정보 jstatus 코드 → 문구. xingAPI 관례이며 실서버 관측 후 보정한다.
var jifStatus = map[string]string{
	"11": "장전",   // 장전 동시호가 시작
	"21": "장중",   // 장 시작
	"31": "동시호가", // 장 마감 동시호가 시작
	"41": "장마감",  // 장 마감
	"51": "시간외",  // 시간외 종가 매매 시작
	"52": "장마감",  // 시간외 종가 매매 종료
	"61": "시간외",  // 시간외 단일가 매매 시작
	"62": "장마감",  // 시간외 단일가 매매 종료
}

// FromJIF 는 코드가 표에 있으면 문구와 true.
func FromJIF(code string) (string, bool) {
	s, ok := jifStatus[code]
	return s, ok
}
