package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var kst = time.FixedZone("KST", 9*3600)

const tokenExpiryLayout = "2006-01-02 15:04:05"

type cachedToken struct {
	AccessToken string `json:"access_token"`
	Expired     string `json:"expired"` // KST, tokenExpiryLayout
	BaseURL     string `json:"base_url"`
}

func (t cachedToken) validAt(now time.Time) bool {
	exp, err := time.ParseInLocation(tokenExpiryLayout, t.Expired, kst)
	if err != nil {
		return false
	}
	return now.Add(10 * time.Minute).Before(exp)
}

// Token 은 캐시 파일의 토큰이 유효하면 그대로 쓰고, 아니면 새로 발급해 저장한다.
// 재발급은 분당 1회 제한이 있으므로 매 호출마다 발급하면 안 된다.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.tok != nil && c.tok.validAt(now) {
		return c.tok.AccessToken, nil
	}
	// base_url 이 다르면(또는 없으면, 구 형식) 캐시를 무시하고 새로 발급한다 — 시세용/매매용, 모의/실전
	// 캐시 파일이 섞여 쓰이는 걸 막는다.
	if t, err := c.readTokenCache(); err == nil && t.BaseURL == c.BaseURL && t.validAt(now) {
		c.tok = &t
		return t.AccessToken, nil
	}
	t, err := c.issueToken(ctx)
	if err != nil {
		return "", err
	}
	if err := c.writeTokenCache(t); err != nil {
		return "", err
	}
	c.tok = &t
	return t.AccessToken, nil
}

// invalidateToken 은 401 을 받았을 때 다음 Token() 이 재발급하도록 만든다.
func (c *Client) invalidateToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tok = nil
	os.Remove(c.TokenCachePath)
}

func (c *Client) issueToken(ctx context.Context) (cachedToken, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "client_credentials", "appkey": c.AppKey, "appsecret": c.AppSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/oauth2/tokenP", bytes.NewReader(body))
	if err != nil {
		return cachedToken{}, err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return cachedToken{}, fmt.Errorf("kis: token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return cachedToken{}, fmt.Errorf("kis: token HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	var r struct {
		AccessToken string `json:"access_token"`
		Expired     string `json:"access_token_token_expired"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.AccessToken == "" {
		return cachedToken{}, fmt.Errorf("kis: token response: %s", bytes.TrimSpace(raw))
	}
	return cachedToken{AccessToken: r.AccessToken, Expired: r.Expired}, nil
}

func (c *Client) readTokenCache() (cachedToken, error) {
	raw, err := os.ReadFile(c.TokenCachePath)
	if err != nil {
		return cachedToken{}, err
	}
	var t cachedToken
	if err := json.Unmarshal(raw, &t); err != nil {
		return cachedToken{}, err
	}
	if t.AccessToken == "" {
		return cachedToken{}, errors.New("empty token cache")
	}
	return t, nil
}

func (c *Client) writeTokenCache(t cachedToken) error {
	t.BaseURL = c.BaseURL
	if err := os.MkdirAll(filepath.Dir(c.TokenCachePath), 0o755); err != nil {
		return err
	}
	raw, _ := json.Marshal(t)
	return os.WriteFile(c.TokenCachePath, raw, 0o600)
}
