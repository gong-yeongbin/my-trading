// Package ls 는 LS증권 Open API 의 실시간 웹소켓 클라이언트다. 뉴스 제목과 업종 지수를 받아 Event 채널로 내보낸다.
// 이 패키지는 TUI 와 config 를 모른다.
package ls

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	BaseURL    string // https://openapi.ls-sec.co.kr:8080
	WSURL      string // wss://openapi.ls-sec.co.kr:9443/websocket
	AppKey     string
	AppSecret  string
	TokenCache string // 토큰 캐시 파일 경로
}

type Client struct {
	cfg  Config
	http *http.Client
	log  *slog.Logger

	mu        sync.Mutex
	token     string
	expiresAt time.Time

	retryMin, retryMax        time.Duration // 재연결 대기. 테스트에서 줄인다.
	pingInterval, pingTimeout time.Duration // 핑 주기·타임아웃. 테스트에서 줄인다.
}

func New(cfg Config, logger *slog.Logger) *Client {
	return &Client{
		cfg:          cfg,
		http:         &http.Client{Timeout: 10 * time.Second},
		log:          logger,
		retryMin:     5 * time.Second,
		retryMax:     60 * time.Second,
		pingInterval: 20 * time.Second,
		pingTimeout:  10 * time.Second,
	}
}

// Event 는 Run 이 채널로 내보내는 값. News, Index, Connected, Disconnected 중 하나.
type Event interface{ isEvent() }

// News 는 NWS 실시간 뉴스 제목 한 건. 출처 필드는 API 에 없다.
type News struct {
	Date, Time, ID, Title, Code string
}

// Index 는 IJ_ 업종지수 한 건. Code 는 업종코드(001 코스피, 301 코스닥). ChangePct 는 0.008 = +0.8%.
type Index struct {
	Code      string
	Value     float64
	Change    float64
	ChangePct float64
	Time      string
}

// Connected 는 웹소켓 연결·구독이 끝났을 때, Disconnected 는 연결이 끊겼을 때 한 번 보낸다.
type Connected struct{}
type Disconnected struct{ Err error }

func (News) isEvent()         {}
func (Index) isEvent()        {}
func (Connected) isEvent()    {}
func (Disconnected) isEvent() {}
