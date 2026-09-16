package ls

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// subscribeMsg 는 클라이언트가 보내는 구독 메시지 (테스트에서 디코딩용).
type subscribeMsg struct {
	Header struct {
		Token  string `json:"token"`
		TrType string `json:"tr_type"`
	} `json:"header"`
	Body struct {
		TrCd  string `json:"tr_cd"`
		TrKey string `json:"tr_key"`
	} `json:"body"`
}

// wsServer 는 토큰 엔드포인트와 웹소켓을 함께 제공하는 가짜 서버.
// onConn 은 연결마다 호출되며 구독 메시지 nsubs 개를 읽은 뒤의 동작을 정의한다.
type wsServer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	subs    [][]subscribeMsg // 연결별 받은 구독
	accepts int
	onConn  func(conn *websocket.Conn, n int)
	nsubs   int
}

func newWSServer(t *testing.T, nsubs int, onConn func(conn *websocket.Conn, n int)) *wsServer {
	t.Helper()
	s := &wsServer{onConn: onConn, nsubs: nsubs}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	// 핸들러는 서버 고루틴에서 돌고 테스트 종료 뒤에도 살아 있을 수 있으므로 t 를 호출하지 않는다. 검증은 subs 기록으로 한다.
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		s.mu.Lock()
		s.accepts++
		n := s.accepts
		s.subs = append(s.subs, nil)
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := 0; i < s.nsubs; i++ {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var sm subscribeMsg
			if err := json.Unmarshal(data, &sm); err != nil {
				return
			}
			s.mu.Lock()
			s.subs[n-1] = append(s.subs[n-1], sm)
			s.mu.Unlock()
		}
		s.onConn(conn, n)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *wsServer) client(t *testing.T) *Client {
	t.Helper()
	c := New(Config{
		BaseURL: s.srv.URL, WSURL: strings.Replace(s.srv.URL, "http://", "ws://", 1) + "/websocket",
		AppKey: "k", AppSecret: "s", TokenCache: filepath.Join(t.TempDir(), "t.json"),
	}, slog.New(slog.DiscardHandler))
	c.retryMin, c.retryMax = 20*time.Millisecond, 50*time.Millisecond
	return c
}

var testSubs = []Subscription{{"NWS", "NWS001"}, {"IJ_", "001"}, {"IJ_", "301"}}

// send 는 서버 → 클라이언트 전송. 클라이언트가 먼저 끊었으면 오류를 무시한다 (테스트 종료 후 실행될 수 있음).
func send(conn *websocket.Conn, raw string) {
	_ = conn.Write(context.Background(), websocket.MessageText, []byte(raw))
}

// next 는 events 에서 다음 이벤트를 꺼낸다. 2초 안에 없으면 실패.
func next(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event within 2s")
		return nil
	}
}

func TestSubscribeAndReceive(t *testing.T) {
	s := newWSServer(t, len(testSubs), func(conn *websocket.Conn, n int) {
		send(conn, `{"header":{"tr_cd":"NWS","rsp_cd":"00000"}}`)
		send(conn, `{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"title":"제목1","date":"20260916","time":"1","id":"a","code":""}}`)
		send(conn, `{"header":{"tr_cd":"IJ_","tr_key":"301"},"body":{"upcode":"301","jisu":"782.15","drate":"0.40","change":"3.10","sign":"5","time":"143210"}}`)
		time.Sleep(200 * time.Millisecond) // 클라이언트가 읽을 시간
		conn.Close(websocket.StatusNormalClosure, "bye")
	})
	c := s.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 16)
	done := make(chan struct{})
	go func() { c.Run(ctx, testSubs, events); close(done) }()

	if _, ok := next(t, events).(Connected); !ok {
		t.Fatal("first event should be Connected")
	}
	n, ok := next(t, events).(News)
	if !ok || n.Title != "제목1" {
		t.Fatalf("expected News, got %+v", n)
	}
	ix, ok := next(t, events).(Index)
	if !ok || ix.Code != "301" || !near(ix.ChangePct, -0.004) {
		t.Fatalf("expected Index 301 -0.4%%, got %+v", ix)
	}
	if _, ok := next(t, events).(Disconnected); !ok {
		t.Fatal("expected Disconnected after server close")
	}

	s.mu.Lock()
	got := s.subs[0]
	s.mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("subscriptions received = %d", len(got))
	}
	for i, sm := range got {
		if sm.Header.Token != "tok" || sm.Header.TrType != "3" {
			t.Errorf("sub %d header = %+v", i, sm.Header)
		}
		if sm.Body.TrCd != testSubs[i].TrCd || sm.Body.TrKey != testSubs[i].TrKey {
			t.Errorf("sub %d body = %+v, want %+v", i, sm.Body, testSubs[i])
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestReconnectResubscribes(t *testing.T) {
	s := newWSServer(t, len(testSubs), func(conn *websocket.Conn, n int) {
		if n == 1 {
			conn.Close(websocket.StatusNormalClosure, "first drop")
			return
		}
		send(conn, `{"header":{"tr_cd":"NWS","tr_key":"NWS001"},"body":{"title":"재연결 후"}}`)
		time.Sleep(200 * time.Millisecond)
		conn.Close(websocket.StatusNormalClosure, "")
	})
	c := s.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 16)
	go c.Run(ctx, testSubs, events)

	var titles []string
	var connects, disconnects int
	deadline := time.After(3 * time.Second)
	for len(titles) == 0 {
		select {
		case ev := <-events:
			switch e := ev.(type) {
			case Connected:
				connects++
			case Disconnected:
				disconnects++
			case News:
				titles = append(titles, e.Title)
			}
		case <-deadline:
			t.Fatalf("no news after reconnect (connects=%d disconnects=%d)", connects, disconnects)
		}
	}
	if connects < 2 || disconnects < 1 {
		t.Errorf("connects=%d disconnects=%d, want >=2 and >=1", connects, disconnects)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subs) < 2 || len(s.subs[1]) != 3 {
		t.Errorf("second connection should receive 3 subscriptions, got %v", s.subs)
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	c := &Client{retryMin: 5 * time.Second, retryMax: 60 * time.Second}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
	d := c.retryMin
	for i, w := range want {
		if d != w {
			t.Errorf("step %d = %v, want %v", i, d, w)
		}
		d = c.nextDelay(d)
	}
}

func TestBackoffResetsAfterConnectedSession(t *testing.T) {
	c := &Client{retryMin: 5 * time.Second, retryMax: 60 * time.Second}
	cases := []struct {
		delay       time.Duration
		connected   bool
		wantWaitFor time.Duration
		wantNext    time.Duration
	}{
		{5 * time.Second, false, 5 * time.Second, 10 * time.Second},
		{10 * time.Second, false, 10 * time.Second, 20 * time.Second},
		{20 * time.Second, true, 5 * time.Second, 5 * time.Second},
	}
	for _, tc := range cases {
		waitFor, next := c.afterSession(tc.delay, tc.connected)
		if waitFor != tc.wantWaitFor || next != tc.wantNext {
			t.Errorf("afterSession(%v, %v) = (%v, %v), want (%v, %v)", tc.delay, tc.connected, waitFor, next, tc.wantWaitFor, tc.wantNext)
		}
	}
}

func TestPingTimeoutClosesConnection(t *testing.T) {
	s := newWSServer(t, len(testSubs), func(conn *websocket.Conn, n int) {
		// 구독 이후 더 이상 Read 하지 않는다. coder/websocket 서버는 Read 중에만 핑에 퐁으로 응답하므로
		// 핑이 응답을 받지 못해 타임아웃된다.
		time.Sleep(3 * time.Second)
	})
	c := s.client(t)
	c.pingInterval = 50 * time.Millisecond
	c.pingTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 16)
	done := make(chan struct{})
	go func() { c.Run(ctx, testSubs, events); close(done) }()

	if _, ok := next(t, events).(Connected); !ok {
		t.Fatal("first event should be Connected")
	}
	if _, ok := next(t, events).(Disconnected); !ok {
		t.Fatal("expected Disconnected after ping timeout")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunStopsOnCancelWhileWaiting(t *testing.T) {
	// 토큰 서버가 없으므로 연결 실패 → 대기 중 취소.
	c := New(Config{BaseURL: "http://127.0.0.1:1", WSURL: "ws://127.0.0.1:1/websocket", AppKey: "k", AppSecret: "s", TokenCache: filepath.Join(t.TempDir(), "t.json")}, slog.New(slog.DiscardHandler))
	c.retryMin, c.retryMax = time.Hour, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 4)
	done := make(chan struct{})
	go func() { c.Run(ctx, testSubs, events); close(done) }()
	if _, ok := next(t, events).(Disconnected); !ok {
		t.Fatal("expected Disconnected on connect failure")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel during backoff wait")
	}
}
