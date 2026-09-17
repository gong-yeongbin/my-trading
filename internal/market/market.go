// Package market 은 국내 주식 장 상태를 판정한다. 시계 기준 판정과 LS 장운영정보(JIF) 코드표를 제공한다.
// 공휴일은 반영하지 않는다 (JIF 가 오면 그 값이 우선).
package market

import "time"

// KST 는 tzdata 없이도 동작하도록 고정 오프셋으로 둔다.
var KST = time.FixedZone("KST", 9*60*60)

// Status 는 장 상태. 문자열이 아니라 타입으로 두어 폴링 시간대 판정 등에서 문구 비교를 하지 않게 한다.
type Status int

const (
	Closed         Status = iota // 휴장
	PreOpen                      // 장전
	Open                         // 장중
	ClosingAuction               // 동시호가
	AfterClose                   // 장마감
	AfterHours                   // 시간외
)

var statusNames = [...]string{"휴장", "장전", "장중", "동시호가", "장마감", "시간외"}

func (s Status) String() string {
	if s < 0 || int(s) >= len(statusNames) {
		return "알 수 없음"
	}
	return statusNames[s]
}

// Trading 은 정규장 거래 시간대(장중·동시호가)인지. 잔고 폴링 등에 쓴다.
func (s Status) Trading() bool { return s == Open || s == ClosingAuction }

// At 은 t 시각의 장 상태 (KST, 공휴일 미반영).
func At(t time.Time) Status {
	t = t.In(KST)
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return Closed
	}
	hm := t.Hour()*60 + t.Minute()
	switch {
	case hm < 9*60:
		return PreOpen
	case hm < 15*60+20:
		return Open
	case hm < 15*60+30:
		return ClosingAuction
	case hm < 16*60:
		return AfterClose
	case hm < 18*60:
		return AfterHours
	default:
		return AfterClose
	}
}

// jifStatus 는 LS 장운영정보 jstatus 코드 → 장 상태. xingAPI 관례이며 실서버 관측 후 보정한다.
var jifStatus = map[string]Status{
	"11": PreOpen,        // 장전 동시호가 시작
	"21": Open,           // 장 시작
	"31": ClosingAuction, // 장 마감 동시호가 시작
	"41": AfterClose,     // 장 마감
	"51": AfterHours,     // 시간외 종가 매매 시작
	"52": AfterClose,     // 시간외 종가 매매 종료
	"61": AfterHours,     // 시간외 단일가 매매 시작
	"62": AfterClose,     // 시간외 단일가 매매 종료
}

// FromJIF 는 코드가 표에 있으면 상태와 true.
func FromJIF(code string) (Status, bool) {
	s, ok := jifStatus[code]
	return s, ok
}
