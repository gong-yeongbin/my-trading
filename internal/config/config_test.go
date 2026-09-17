package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodYAML = `
db_path: data/test.db
kis:
  trade_env: demo
  token_cache: data/token.json
  requests_per_second: 0
  balance_poll_seconds: 10
ls:
  base_url: https://ls.example
  ws_url: wss://ls.example/websocket
  token_cache: data/ls_token.json
log:
  file: data/test.log
universe:
  markets: [kospi, kosdaq]
  group_codes: [ST]
  exclude_flags: [거래정지, SPAC]
fetch:
  start_date: "2021-01-01"
  daily_at: "04:00"
strategy:
  index_ma_days: 20
  min_turnover: 10000000000
  turnover_ma_days: 20
  turnover_ratio_min: 3.0
  close_to_high_min: 0.99
  change_min: 0.03
  change_max: 0.20
  ma_short_days: 20
  ma_long_days: 60
  new_high_days: 20
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// clearKISEnv 는 KIS_* 환경변수를 전부 비운다. t.Setenv 는 테스트가 끝나면 원래대로 되돌린다.
func clearKISEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"KIS_APP_KEY", "KIS_APP_SECRET", "KIS_ACCOUNT",
		"KIS_DEMO_APP_KEY", "KIS_DEMO_APP_SECRET", "KIS_DEMO_ACCOUNT",
		"KIS_REAL_APP_KEY", "KIS_REAL_APP_SECRET", "KIS_REAL_ACCOUNT",
	} {
		t.Setenv(v, "")
	}
}

func TestLoadGood(t *testing.T) {
	clearKISEnv(t)
	t.Setenv("KIS_APP_KEY", "k")
	t.Setenv("KIS_APP_SECRET", "s")
	t.Setenv("KIS_ACCOUNT", "12345678-01")
	t.Setenv("LS_APP_KEY", "lk")
	t.Setenv("LS_APP_SECRET", "ls")
	cfg, err := Load(writeTemp(t, "c.yaml", goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != "data/test.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.KIS.MarketBaseURL() != RealBaseURL {
		t.Errorf("MarketBaseURL = %q", cfg.KIS.MarketBaseURL())
	}
	if cfg.KIS.TradeBaseURL() != DemoBaseURL {
		t.Errorf("TradeBaseURL = %q", cfg.KIS.TradeBaseURL())
	}
	if cfg.KIS.MarketRPS() != 15 {
		t.Errorf("MarketRPS = %v", cfg.KIS.MarketRPS())
	}
	if cfg.KIS.TradeRPS() != 1.5 {
		t.Errorf("TradeRPS = %v", cfg.KIS.TradeRPS())
	}
	// 시세(Market)는 KIS_REAL_* 만 본다. 여기선 설정하지 않았으므로 비어 있어야 한다.
	if cfg.KIS.Market.AppKey != "" || cfg.KIS.Market.AppSecret != "" || cfg.KIS.Market.Account != "" {
		t.Errorf("market should not fall back to legacy keys: %+v", cfg.KIS.Market)
	}
	// 매매(Trade, demo)는 구 이름으로 fallback 한다.
	if cfg.KIS.Trade.AppKey != "k" || cfg.KIS.Trade.AppSecret != "s" || cfg.KIS.Trade.Account != "12345678-01" {
		t.Errorf("trade legacy fallback: %+v", cfg.KIS.Trade)
	}
	if cfg.KIS.BalancePollSeconds != 10 {
		t.Errorf("BalancePollSeconds = %d", cfg.KIS.BalancePollSeconds)
	}
	// trade_token_cache 가 yaml 에 없으면 token_cache + ".trade" 가 기본값이다.
	if cfg.KIS.TradeTokenCache != cfg.KIS.TokenCache+".trade" {
		t.Errorf("TradeTokenCache default = %q, want %q", cfg.KIS.TradeTokenCache, cfg.KIS.TokenCache+".trade")
	}
	if cfg.LS.AppKey != "lk" || cfg.LS.AppSecret != "ls" || !cfg.LS.HasAppKey() {
		t.Errorf("ls env not read: %+v", cfg.LS)
	}
	if cfg.LS.WSURL != "wss://ls.example/websocket" {
		t.Errorf("LS.WSURL = %q", cfg.LS.WSURL)
	}
	if cfg.Log.File != "data/test.log" {
		t.Errorf("Log.File = %q", cfg.Log.File)
	}
	if cfg.Strategy.MALongDays != 60 || cfg.Strategy.TurnoverRatioMin != 3.0 {
		t.Errorf("strategy not parsed: %+v", cfg.Strategy)
	}
	if len(cfg.Universe.ExcludeFlags) != 2 {
		t.Errorf("ExcludeFlags = %v", cfg.Universe.ExcludeFlags)
	}
	if cfg.Fetch.DailyAt != "04:00" {
		t.Errorf("DailyAt = %q", cfg.Fetch.DailyAt)
	}
}

func TestLSHasAppKeyFalseWhenMissing(t *testing.T) {
	t.Setenv("LS_APP_KEY", "")
	t.Setenv("LS_APP_SECRET", "")
	cfg, err := Load(writeTemp(t, "c.yaml", goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LS.HasAppKey() {
		t.Error("HasAppKey should be false without env")
	}
}

func TestTradeEnvDefaults(t *testing.T) {
	k := KISConfig{TradeEnv: "real"}
	if k.TradeBaseURL() != RealBaseURL || k.TradeRPS() != 15 {
		t.Errorf("real defaults wrong: %q %v", k.TradeBaseURL(), k.TradeRPS())
	}
	k2 := KISConfig{TradeEnv: "demo"}
	if k2.TradeBaseURL() != DemoBaseURL || k2.TradeRPS() != 1.5 {
		t.Errorf("demo defaults wrong: %q %v", k2.TradeBaseURL(), k2.TradeRPS())
	}
	// MarketBaseURL/MarketRPS 는 TradeEnv 와 무관하게 항상 실전.
	if k2.MarketBaseURL() != RealBaseURL {
		t.Errorf("MarketBaseURL should always be real: %q", k2.MarketBaseURL())
	}
	k2.RequestsPerSecond = 3
	if k2.MarketRPS() != 3 {
		t.Errorf("explicit rps ignored")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]string{
		"trade_env": "trade_env: demo",
		"poll":      "balance_poll_seconds: 10",
		"ws":        "ws_url: wss://ls.example/websocket",
		"logfile":   "file: data/test.log",
		"ma_order":  "ma_short_days: 20\n  ma_long_days: 60",
		"change":    "change_min: 0.03\n  change_max: 0.20",
		"start":     `start_date: "2021-01-01"`,
	}
	bad := map[string]string{
		"trade_env": "trade_env: paper",
		"poll":      "balance_poll_seconds: 0",
		"ws":        `ws_url: ""`,
		"logfile":   `file: ""`,
		"ma_order":  "ma_short_days: 60\n  ma_long_days: 20",
		"change":    "change_min: 0.30\n  change_max: 0.20",
		"start":     `start_date: "2021/01/01"`,
	}
	for name := range cases {
		y := strings.Replace(goodYAML, cases[name], bad[name], 1)
		if y == goodYAML {
			t.Fatalf("%s: replacement did not apply", name)
		}
		if _, err := Load(writeTemp(t, "c.yaml", y)); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestRequireMarketKey(t *testing.T) {
	cfg := &Config{}
	if err := cfg.RequireMarketKey(); err == nil {
		t.Error("expected error when market app key missing")
	}
	cfg.KIS.Market.AppKey, cfg.KIS.Market.AppSecret = "a", "b"
	if err := cfg.RequireMarketKey(); err != nil {
		t.Error(err)
	}
}

func TestRequireTradeKey(t *testing.T) {
	cfg := &Config{}
	if err := cfg.RequireTradeKey(); err == nil {
		t.Error("expected error when trade app key missing")
	}
	cfg.KIS.Trade.AppKey, cfg.KIS.Trade.AppSecret = "a", "b"
	if err := cfg.RequireTradeKey(); err != nil {
		t.Error(err)
	}
}

func TestLoadDotEnv(t *testing.T) {
	p := writeTemp(t, ".env", "# comment\nKIS_APP_KEY=abc\nKIS_APP_SECRET=\"quoted\"\nEXISTING=fromfile\n")
	t.Setenv("KIS_APP_KEY", "")
	t.Setenv("KIS_APP_SECRET", "")
	t.Setenv("EXISTING", "already")
	if err := LoadDotEnv(p); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("KIS_APP_KEY") != "abc" {
		t.Errorf("KIS_APP_KEY = %q", os.Getenv("KIS_APP_KEY"))
	}
	if os.Getenv("KIS_APP_SECRET") != "quoted" {
		t.Errorf("quotes not stripped: %q", os.Getenv("KIS_APP_SECRET"))
	}
	if os.Getenv("EXISTING") != "already" {
		t.Errorf("existing env var overwritten")
	}
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("missing file should not error: %v", err)
	}
}

func TestFetchDailyAtDefaultAndValidation(t *testing.T) {
	y := strings.Replace(goodYAML, "  daily_at: \"04:00\"\n", "", 1)
	cfg, err := Load(writeTemp(t, "c.yaml", y))
	if err != nil || cfg.Fetch.DailyAt != "04:00" {
		t.Fatalf("missing daily_at should default to 04:00: %q %v", cfg.Fetch.DailyAt, err)
	}
	for _, bad := range []string{`"4:00"`, `"24:00"`, `"04:60"`, `"abc"`} {
		y := strings.Replace(goodYAML, `daily_at: "04:00"`, "daily_at: "+bad, 1)
		if _, err := Load(writeTemp(t, "c.yaml", y)); err == nil {
			t.Errorf("daily_at %s should fail validation", bad)
		}
	}
}

// TestLoadMarketAlwaysRealAndTradeByEnv: Market 은 trade_env 와 무관하게 항상 KIS_REAL_*.
// Trade 는 trade_env 에 따라 KIS_DEMO_* 또는 KIS_REAL_*.
func TestLoadMarketAlwaysRealAndTradeByEnv(t *testing.T) {
	clearKISEnv(t)
	t.Setenv("KIS_DEMO_APP_KEY", "dk")
	t.Setenv("KIS_DEMO_APP_SECRET", "ds")
	t.Setenv("KIS_DEMO_ACCOUNT", "11111111-01")
	t.Setenv("KIS_REAL_APP_KEY", "rk")
	t.Setenv("KIS_REAL_APP_SECRET", "rs")
	t.Setenv("KIS_REAL_ACCOUNT", "22222222-01")

	cfg, err := Load(writeTemp(t, "c.yaml", goodYAML)) // trade_env: demo
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.Market.AppKey != "rk" || cfg.KIS.Market.AppSecret != "rs" || cfg.KIS.Market.Account != "22222222-01" {
		t.Errorf("market keys (demo trade_env): %+v", cfg.KIS.Market)
	}
	if cfg.KIS.Trade.AppKey != "dk" || cfg.KIS.Trade.AppSecret != "ds" || cfg.KIS.Trade.Account != "11111111-01" {
		t.Errorf("trade keys (demo): %+v", cfg.KIS.Trade)
	}

	real := strings.Replace(goodYAML, "trade_env: demo", "trade_env: real", 1)
	cfg, err = Load(writeTemp(t, "c.yaml", real))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.Market.AppKey != "rk" || cfg.KIS.Market.AppSecret != "rs" || cfg.KIS.Market.Account != "22222222-01" {
		t.Errorf("market keys (real trade_env): %+v", cfg.KIS.Market)
	}
	if cfg.KIS.Trade.AppKey != "rk" || cfg.KIS.Trade.AppSecret != "rs" || cfg.KIS.Trade.Account != "22222222-01" {
		t.Errorf("trade keys (real) should equal market: %+v", cfg.KIS.Trade)
	}
}

// TestLoadFallsBackToLegacyKeys: 구 이름(KIS_APP_KEY 등)은 Trade(demo)만 fallback 하고, Market 은 하지 않는다.
func TestLoadFallsBackToLegacyKeys(t *testing.T) {
	clearKISEnv(t)
	t.Setenv("KIS_APP_KEY", "lk")
	t.Setenv("KIS_APP_SECRET", "ls")
	t.Setenv("KIS_ACCOUNT", "33333333-01")
	cfg, err := Load(writeTemp(t, "c.yaml", goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.Trade.AppKey != "lk" || cfg.KIS.Trade.AppSecret != "ls" || cfg.KIS.Trade.Account != "33333333-01" {
		t.Errorf("legacy fallback: %+v", cfg.KIS.Trade)
	}
	if cfg.KIS.Market.AppKey != "" || cfg.KIS.Market.AppSecret != "" || cfg.KIS.Market.Account != "" {
		t.Errorf("market must not fall back to legacy keys: %+v", cfg.KIS.Market)
	}
}

// TestLoadMigratesLegacyEnvKey: trade_env 가 없고 구 이름 env 만 있으면 그 값을 TradeEnv 로 쓴다.
func TestLoadMigratesLegacyEnvKey(t *testing.T) {
	y := strings.Replace(goodYAML, "trade_env: demo", "env: real", 1)
	cfg, err := Load(writeTemp(t, "c.yaml", y))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.TradeEnv != "real" {
		t.Errorf("TradeEnv should migrate from legacy env: %q", cfg.KIS.TradeEnv)
	}
}

// TestLoadTradeEnvBeatsLegacyEnvKey: trade_env 와 구 이름 env 가 둘 다 있으면 trade_env 가 이긴다.
func TestLoadTradeEnvBeatsLegacyEnvKey(t *testing.T) {
	y := strings.Replace(goodYAML, "trade_env: demo", "trade_env: real\n  env: demo", 1)
	cfg, err := Load(writeTemp(t, "c.yaml", y))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.TradeEnv != "real" {
		t.Errorf("trade_env should win over legacy env: %q", cfg.KIS.TradeEnv)
	}
}
