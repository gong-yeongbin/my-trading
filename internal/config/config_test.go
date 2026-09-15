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
  env: demo
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

func TestLoadGood(t *testing.T) {
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
	if cfg.KIS.BaseURL() != DemoBaseURL {
		t.Errorf("BaseURL = %q", cfg.KIS.BaseURL())
	}
	if cfg.KIS.EffectiveRPS() != 1.5 {
		t.Errorf("EffectiveRPS = %v", cfg.KIS.EffectiveRPS())
	}
	if cfg.KIS.AppKey != "k" || cfg.KIS.AppSecret != "s" || cfg.KIS.Account != "12345678-01" {
		t.Errorf("kis env not read: %+v", cfg.KIS)
	}
	if cfg.KIS.BalancePollSeconds != 10 {
		t.Errorf("BalancePollSeconds = %d", cfg.KIS.BalancePollSeconds)
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

func TestRealEnvDefaults(t *testing.T) {
	k := KISConfig{Env: "real"}
	if k.BaseURL() != RealBaseURL || k.EffectiveRPS() != 15 {
		t.Errorf("real defaults wrong: %q %v", k.BaseURL(), k.EffectiveRPS())
	}
	k.RequestsPerSecond = 3
	if k.EffectiveRPS() != 3 {
		t.Errorf("explicit rps ignored")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]string{
		"env":      "env: demo",
		"poll":     "balance_poll_seconds: 10",
		"ws":       "ws_url: wss://ls.example/websocket",
		"logfile":  "file: data/test.log",
		"ma_order": "ma_short_days: 20\n  ma_long_days: 60",
		"change":   "change_min: 0.03\n  change_max: 0.20",
		"start":    `start_date: "2021-01-01"`,
	}
	bad := map[string]string{
		"env":      "env: paper",
		"poll":     "balance_poll_seconds: 0",
		"ws":       `ws_url: ""`,
		"logfile":  `file: ""`,
		"ma_order": "ma_short_days: 60\n  ma_long_days: 20",
		"change":   "change_min: 0.30\n  change_max: 0.20",
		"start":    `start_date: "2021/01/01"`,
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

func TestRequireAppKey(t *testing.T) {
	cfg := &Config{}
	if err := cfg.RequireAppKey(); err == nil {
		t.Error("expected error when app key missing")
	}
	cfg.KIS.AppKey, cfg.KIS.AppSecret = "a", "b"
	if err := cfg.RequireAppKey(); err != nil {
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
