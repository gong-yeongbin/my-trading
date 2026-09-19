// Package tui 는 bubbletea 전체 화면 하나를 그린다. 계산은 하지 않고 메시지로 받은 값만 표시한다.
package tui

import (
	"time"

	"github.com/gong-yeongbin/my-trading/internal/market"
	"github.com/gong-yeongbin/my-trading/internal/settings"
)

// WatchRow 는 관심종목 패널 한 줄. 7단계에서 screener.WatchItem 을 여기로 옮긴다.
type WatchRow struct {
	Name, Market string
	PrevClose    int64
	MinClose     int64
	MinChangePct float64 // 0.031 = +3.1%
	MinVolume    int64
	AvgTurnover  int64 // 평균 거래대금 (원)
}

// MarketFilter 는 시장별 진입 가능 여부 문구: "진입가능", "차단", "알 수 없음".
type MarketFilter struct {
	Kospi, Kosdaq string
}

// HoldingRow 는 보유종목 패널 한 줄. HoldDays 가 0 이면 "-" 로 표시.
type HoldingRow struct {
	Name, Market string
	Qty          int64
	AvgPrice     int64
	Price        int64
	PnL          int64
	PnLPct       float64
	HoldDays     int
}

type HoldingsSummary struct {
	Total  int64 // 평가금액
	PnL    int64
	PnLPct float64
	Cash   int64
}

type LogLine struct {
	Time time.Time
	Kind string // 수집 / 지수 / 매매 / 연결 / 오류
	Msg  string
}

// 아래는 외부(데이터 소스)가 보내는 메시지. 전부 값 타입이라 그대로 tea.Msg 로 쓴다.

type NewsMsg struct {
	Source, Title string
}

// IndexMsg 의 Market 은 "kospi" 또는 "kosdaq". ChangePct 는 0.008 = +0.8%.
type IndexMsg struct {
	Market    string
	Value     float64
	ChangePct float64
}

// IndexPrevCloseMsg 는 저장소의 지수 전일 종가. 실시간 값이 없을 때 "(전일)" 로 보인다.
type IndexPrevCloseMsg struct {
	Market string
	Close  float64
}

type WatchMsg struct {
	AsOf   string // "09-12"
	Rows   []WatchRow
	Filter MarketFilter
}

// HoldingsMsg 의 Connected 가 false 면 "미연결" 로 표시한다.
type HoldingsMsg struct {
	Connected bool
	At        time.Time // 조회 시각. 폴링이 없으니 언제 값인지 보여준다
	Rows      []HoldingRow
	Summary   HoldingsSummary
}

// LogMsg 는 최신순으로 정렬된 전체 목록을 담는다 (증분 아님).
type LogMsg struct {
	Lines []LogLine
}

// SettingsMsg 는 시작 시 읽은 설정 값. Err 가 있으면 패널에 오류를 보인다.
type SettingsMsg struct {
	Values settings.Values
	Err    error
}

// FetchStatusMsg 는 자동 일봉 수집의 진행 상태. Running 이 false 면 하단 표시를 지운다.
type FetchStatusMsg struct {
	Running     bool
	Done, Total int
}

// MarketStatusMsg 는 LS 장운영정보를 장 상태로 바꾼 것. 그날 안에서는 시계 판정보다 우선한다.
type MarketStatusMsg struct {
	Status market.Status
}

// ConnectedMsg 는 실시간 연결(LS)이 되어 구독까지 끝났을 때. 데이터가 오기 전까지 "연결됨 · 대기" 로 보인다.
type ConnectedMsg struct{}

// DisconnectedMsg 는 실시간 연결(LS)이 끊겼을 때. 뉴스·지수를 미연결로 되돌린다.
type DisconnectedMsg struct{}
