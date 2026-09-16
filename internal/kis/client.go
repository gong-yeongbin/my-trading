// Package kis 는 한국투자증권 Open API 클라이언트다.
package kis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	rateLimitMsgCd = "EGW00201"
	maxRateRetries = 3
)

// ErrUnauthorized 는 토큰 재발급 후에도 401 이 나서 포기할 때 반환된다 (spec §11).
var ErrUnauthorized = errors.New("kis: 인증 실패 (앱키 또는 토큰)")

type Client struct {
	BaseURL        string
	AppKey         string
	AppSecret      string
	TokenCachePath string
	HTTP           *http.Client
	MasterBaseURL  string // 비어 있으면 DefaultMasterBaseURL

	limiter   *limiter
	retryWait time.Duration
	now       func() time.Time

	mu  sync.Mutex
	tok *cachedToken
}

func New(baseURL, appKey, appSecret, tokenCachePath string, rps float64) *Client {
	return &Client{
		BaseURL:        baseURL,
		AppKey:         appKey,
		AppSecret:      appSecret,
		TokenCachePath: tokenCachePath,
		HTTP:           &http.Client{Timeout: 30 * time.Second},
		limiter:        newLimiter(rps),
		retryWait:      time.Second,
		now:            time.Now,
	}
}

type envelope struct {
	RtCd  string `json:"rt_cd"`
	MsgCd string `json:"msg_cd"`
	Msg1  string `json:"msg1"`
}

// get 은 GET 호출을 수행하고 out 에 응답 JSON 을 넣는다.
// 401 은 토큰을 한 번 재발급해 재시도, EGW00201 은 retryWait 후 최대 maxRateRetries 회 재시도한다.
func (c *Client) get(ctx context.Context, path, trID string, params url.Values, out any) error {
	refreshed := false
	rateRetries := 0
	for {
		tok, err := c.Token(ctx)
		if err != nil {
			return err
		}
		status, body, err := c.do(ctx, path, trID, params, tok)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized {
			if refreshed {
				return fmt.Errorf("%w: kis: %s: 401 after token refresh: %s", ErrUnauthorized, path, body)
			}
			refreshed = true
			c.invalidateToken()
			continue
		}
		var env envelope
		_ = json.Unmarshal(body, &env)
		if env.MsgCd == rateLimitMsgCd {
			rateRetries++
			if rateRetries > maxRateRetries {
				return fmt.Errorf("kis: %s: rate limit exceeded after %d retries", path, maxRateRetries)
			}
			select {
			case <-time.After(c.retryWait):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		if status != http.StatusOK {
			return fmt.Errorf("kis: %s: HTTP %d: %s", path, status, body)
		}
		if env.RtCd != "0" {
			return fmt.Errorf("kis: %s: %s %s", path, env.MsgCd, env.Msg1)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("kis: %s: decode: %w", path, err)
		}
		return nil
	}
}

func (c *Client) do(ctx context.Context, path, trID string, params url.Values, tok string) (int, []byte, error) {
	if err := c.limiter.wait(ctx); err != nil {
		return 0, nil, err
	}
	u := c.BaseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	req.Header.Set("authorization", "Bearer "+tok)
	req.Header.Set("appkey", c.AppKey)
	req.Header.Set("appsecret", c.AppSecret)
	req.Header.Set("tr_id", trID)
	req.Header.Set("custtype", "P")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("kis: %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("kis: %s: read body: %w", path, err)
	}
	return resp.StatusCode, body, nil
}
