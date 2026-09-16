package ls

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tokenSafety = 30 * time.Second

// cachedToken 은 token_cache 파일 형식.
type cachedToken struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Token 은 유효한 접근 토큰을 돌려준다. 메모리 → 파일 캐시 → 발급 순서.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" {
		c.loadCache()
	}
	if c.token != "" && time.Until(c.expiresAt) > tokenSafety {
		return c.token, nil
	}
	tok, exp, err := c.requestToken(ctx)
	if err != nil {
		return "", err
	}
	c.token, c.expiresAt = tok, exp
	c.saveCache()
	return tok, nil
}

func (c *Client) requestToken(ctx context.Context) (string, time.Time, error) {
	form := url.Values{
		"appkey":       {c.cfg.AppKey},
		"appsecretkey": {c.cfg.AppSecret},
		"grant_type":   {"client_credentials"},
		"scope":        {"oob"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("ls token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("ls token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", time.Time{}, fmt.Errorf("ls token: parse: %w", err)
	}
	if out.AccessToken == "" {
		return "", time.Time{}, errors.New("ls token: access_token 없음")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 86400
	}
	return out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn) * time.Second), nil
}

func (c *Client) loadCache() {
	b, err := os.ReadFile(c.cfg.TokenCache)
	if err != nil {
		return
	}
	var ct cachedToken
	if json.Unmarshal(b, &ct) == nil && ct.AccessToken != "" {
		c.token, c.expiresAt = ct.AccessToken, ct.ExpiresAt
	}
}

func (c *Client) saveCache() {
	b, _ := json.Marshal(cachedToken{AccessToken: c.token, ExpiresAt: c.expiresAt})
	if err := os.MkdirAll(filepath.Dir(c.cfg.TokenCache), 0o700); err != nil {
		c.log.Warn("ls token cache dir", "err", err)
		return
	}
	if err := os.WriteFile(c.cfg.TokenCache, b, 0o600); err != nil {
		c.log.Warn("ls token cache write", "err", err)
	}
}
