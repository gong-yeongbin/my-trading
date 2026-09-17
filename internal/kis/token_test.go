package kis

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestTokenCacheDiscardedWhenBaseURLDiffers: 캐시 파일의 base_url 이 클라이언트와 다르면
// (시세용/매매용, 모의/실전 캐시가 섞여도) 캐시를 버리고 새로 발급한다.
func TestTokenCacheDiscardedWhenBaseURLDiffers(t *testing.T) {
	f := newFakeKIS(t)
	ctx := context.Background()
	stale := cachedToken{
		AccessToken: "stale",
		Expired:     time.Now().In(kst).Add(24 * time.Hour).Format(tokenExpiryLayout),
		BaseURL:     "https://other.example",
	}
	raw, _ := json.Marshal(stale)
	if err := os.WriteFile(f.client.TokenCachePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := f.client.Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-1" {
		t.Errorf("expected fresh token issued, got %q", tok)
	}
	if f.calls() != 1 {
		t.Errorf("token issued %d times, want 1", f.calls())
	}
}

// TestTokenCacheReusedWhenBaseURLMatches: base_url 이 같으면 캐시를 그대로 쓴다.
func TestTokenCacheReusedWhenBaseURLMatches(t *testing.T) {
	f := newFakeKIS(t)
	ctx := context.Background()
	same := cachedToken{
		AccessToken: "cached",
		Expired:     time.Now().In(kst).Add(24 * time.Hour).Format(tokenExpiryLayout),
		BaseURL:     f.client.BaseURL,
	}
	raw, _ := json.Marshal(same)
	if err := os.WriteFile(f.client.TokenCachePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := f.client.Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "cached" {
		t.Errorf("expected cached token reused, got %q", tok)
	}
	if f.calls() != 0 {
		t.Errorf("token issued %d times, want 0", f.calls())
	}
}

// TestTokenCacheDiscardedWhenBaseURLMissing: 구 형식(캐시에 base_url 필드가 없음)은 무조건 버린다.
func TestTokenCacheDiscardedWhenBaseURLMissing(t *testing.T) {
	f := newFakeKIS(t)
	ctx := context.Background()
	old := struct {
		AccessToken string `json:"access_token"`
		Expired     string `json:"expired"`
	}{
		AccessToken: "old-format",
		Expired:     time.Now().In(kst).Add(24 * time.Hour).Format(tokenExpiryLayout),
	}
	raw, _ := json.Marshal(old)
	if err := os.WriteFile(f.client.TokenCachePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := f.client.Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-1" {
		t.Errorf("expected fresh token issued for old-format cache, got %q", tok)
	}
	if f.calls() != 1 {
		t.Errorf("token issued %d times, want 1", f.calls())
	}
}
