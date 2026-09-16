package ls

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

type Subscription struct {
	TrCd, TrKey string
}

// Run 은 ctx 가 끝날 때까지 웹소켓 연결을 유지한다. 연결·구독이 끝나면 Connected, 끊기면 Disconnected 를 보내고
// retryMin 부터 2배씩 retryMax 까지 기다렸다가 다시 연결한다. 연결에 성공했던 뒤에는 대기를 retryMin 으로 되돌린다.
func (c *Client) Run(ctx context.Context, subs []Subscription, events chan<- Event) {
	delay := c.retryMin
	for {
		connected, err := c.session(ctx, subs, events)
		if ctx.Err() != nil {
			return
		}
		waitFor, next := c.afterSession(delay, connected)
		if connected {
			c.log.Warn("ls websocket 끊김", "err", err, "retry_in", waitFor)
		} else {
			c.log.Warn("ls websocket 연결 실패", "err", err, "retry_in", waitFor)
		}
		emit(ctx, events, Disconnected{Err: err})
		if !sleepCtx(ctx, waitFor) {
			return
		}
		delay = next
	}
}

// afterSession 은 session 이 끝난 뒤 이번에 기다릴 시간(waitFor)과 다음 실패에 쓸 대기(next)를 정한다.
// connected 였던 세션 뒤에는 곧바로 retryMin 으로 되돌리고, 그렇지 않으면 delay 를 2배로 늘린다(retryMax 까지).
func (c *Client) afterSession(delay time.Duration, connected bool) (waitFor, next time.Duration) {
	if connected {
		return c.retryMin, c.retryMin
	}
	return delay, c.nextDelay(delay)
}

func (c *Client) nextDelay(d time.Duration) time.Duration {
	return min(d*2, c.retryMax)
}

// session 은 한 번의 연결을 수행한다. connected 는 구독까지 끝났는지.
func (c *Client) session(ctx context.Context, subs []Subscription, events chan<- Event) (connected bool, err error) {
	token, err := c.Token(ctx)
	if err != nil {
		return false, err
	}
	conn, _, err := websocket.Dial(ctx, c.cfg.WSURL, &websocket.DialOptions{HTTPClient: c.http})
	if err != nil {
		return false, err
	}
	defer conn.CloseNow()

	for _, s := range subs {
		msg := map[string]any{
			"header": map[string]string{"token": token, "tr_type": "3"},
			"body":   map[string]string{"tr_cd": s.TrCd, "tr_key": s.TrKey},
		}
		b, _ := json.Marshal(msg)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			return false, err
		}
	}
	c.log.Info("ls websocket 연결", "subs", len(subs))
	emit(ctx, events, Connected{})

	// ping 이 실패하면 연결을 닫아 아래 Read 가 오류로 빠져나오게 한다.
	pctx, cancelPing := context.WithCancel(ctx)
	defer cancelPing()
	go func() {
		t := time.NewTicker(c.pingInterval)
		defer t.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-t.C:
				pingCtx, cancel := context.WithTimeout(pctx, c.pingTimeout)
				err := conn.Ping(pingCtx)
				cancel()
				if err != nil {
					c.log.Warn("ls ping 실패", "err", err)
					conn.CloseNow()
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return true, err
		}
		if ev, ok := parseMessage(data); ok {
			if e, isErr := ev.(SubscribeError); isErr {
				c.log.Warn("ls 구독 거부", "tr_cd", e.TrCd, "rsp_cd", e.Code, "rsp_msg", e.Msg)
			}
			emit(ctx, events, ev)
		}
	}
}

func emit(ctx context.Context, events chan<- Event, ev Event) {
	select {
	case events <- ev:
	case <-ctx.Done():
	}
}

// sleepCtx 는 d 만큼 기다린다. ctx 가 먼저 끝나면 false.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
