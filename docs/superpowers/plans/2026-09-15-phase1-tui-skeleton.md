# 1단계: 모듈 초기화·설정·TUI 뼈대 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `trader`를 인자 없이 실행하면 설계 10.1절 레이아웃(상단 뉴스+시계 / 왼쪽 메뉴 / 가운데 패널 / 하단 지수)이 가짜 데이터로 전체 화면에 뜨고, 키로 메뉴·패널을 오갈 수 있게 한다.

**Architecture:** `config`(설정·.env) → `tui`(bubbletea 모델 하나. 메시지 타입으로 뉴스·지수·관심종목·보유종목·로그를 받아 그리기만 한다) → `cmd/trader`(진입점). 데이터 소스는 전부 나중 단계가 `tea.Msg`로 밀어 넣는다. 이 단계에서는 `fake.go`가 그 자리를 채운다.

**Tech Stack:** Go 1.27, `gopkg.in/yaml.v3`, `github.com/charmbracelet/bubbletea` v1, `github.com/charmbracelet/lipgloss` v1, `github.com/charmbracelet/x/ansi`(lipgloss 의존성, 한글 폭 기준 잘라내기). 테스트는 표준 `testing`만 사용.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (3~5절, 10.1절, 10.4절, 11절의 TUI 항목)

## Global Constraints

- 모듈 경로: `github.com/gong-yeongbin/my-trading`
- 외부 의존성은 위 목록으로 제한. 테스트 프레임워크(testify 등) 추가 금지. `bubbles`는 필요해질 때 추가한다.
- 가격은 `int64` 원 단위, 지수는 `float64`.
- 터미널 최소 100×24. 한글은 2칸 폭이므로 폭 계산은 `lipgloss.Width`, 잘라내기는 `ansi.Truncate`만 쓴다 (`len`, `utf8.RuneCountInString` 금지).
- TUI 패키지는 로직을 갖지 않는다. 계산은 다른 패키지에서 하고 TUI는 메시지를 받아 그린다.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/token.json`은 읽기 금지. 값이 필요하면 프로그램이 읽게 하고 Claude는 열지 않는다.
- 파일 편집 후 `gofmt -l .` 결과가 비어 있어야 한다.
- 실제 터미널 렌더링은 Claude가 볼 수 없다. 마지막 수동 확인은 사용자가 실행하고 결과를 말로 전달한다.

---

## 파일 구조

```
go.mod
config.yaml
cmd/trader/main.go            진입점. 인자 없으면 tui.Run, 그 외는 6단계에서 추가
internal/config/config.go     Config 구조체, Load, Validate
internal/config/dotenv.go     LoadDotEnv
internal/config/config_test.go
internal/tui/types.go         행 타입(WatchRow, HoldingRow, LogLine)과 메시지 타입
internal/tui/model.go         Model, New, Init, Update, 키 처리, 스크롤
internal/tui/view.go          View: 레이아웃, 표 렌더링, 숫자 포맷
internal/tui/fake.go          1단계용 가짜 데이터 (7·8단계에서 삭제)
internal/tui/run.go           Run(ctx, cfg): tea.Program 생성·실행
internal/tui/model_test.go    상태 전이 테스트
internal/tui/view_test.go     렌더링 문구·크기 테스트
```

---

### Task 1: 모듈 초기화와 설정 로딩

**Files:**
- Create: `go.mod`, `config.yaml`, `internal/config/config.go`, `internal/config/dotenv.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load(path string) (*Config, error)`, `config.LoadDotEnv(path string) error`, `(*Config).RequireAppKey() error`, `KISConfig.BaseURL() string`, `KISConfig.EffectiveRPS() float64`, `LSConfig.HasAppKey() bool`. 구조체 필드는 아래 코드 그대로. 4~8단계가 전부 이 구조체를 소비한다.

- [ ] **Step 1: 모듈 초기화**

```bash
cd /Users/gong-yeongbin/orca/projects/my-trading
go mod init github.com/gong-yeongbin/my-trading
go get gopkg.in/yaml.v3@latest
go get github.com/charmbracelet/bubbletea@v1
go get github.com/charmbracelet/lipgloss@v1
```

Expected: `go.mod`에 `module github.com/gong-yeongbin/my-trading`, `go 1.27`, yaml.v3·bubbletea·lipgloss require.

- [ ] **Step 2: config.yaml 작성**

```yaml
db_path: data/market.db

kis:
  env: demo                    # demo: 모의투자 / real: 실전
  token_cache: data/token.json
  requests_per_second: 0       # 0 이면 env 기본값 (demo 1.5, real 15)
  # 앱키는 .env 의 KIS_APP_KEY, KIS_APP_SECRET. 계좌번호는 KIS_ACCOUNT ("12345678-01")
  balance_poll_seconds: 10     # 장중 보유 종목 갱신 주기

ls:
  base_url: https://openapi.ls-sec.co.kr:8080
  ws_url: wss://openapi.ls-sec.co.kr:9443/websocket
  # 앱키는 .env 의 LS_APP_KEY, LS_APP_SECRET. 없으면 뉴스·지수 줄은 "미연결"
  token_cache: data/ls_token.json

log:
  file: data/trader.log

universe:
  markets: [kospi, kosdaq]
  group_codes: [ST]
  exclude_flags: [거래정지, 정리매매, 관리종목, 시장경고, 단기과열, 이상급등, SPAC]

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
```

- [ ] **Step 3: 실패하는 테스트 작성**

`internal/config/config_test.go`:

```go
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
```

- [ ] **Step 4: 테스트 실패 확인**

Run: `go test ./internal/config/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: Load` 등)

- [ ] **Step 5: 구현**

`internal/config/config.go`:

```go
// Package config 는 config.yaml 과 환경변수에서 실행 설정을 읽는다.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	RealBaseURL = "https://openapi.koreainvestment.com:9443"
	DemoBaseURL = "https://openapivts.koreainvestment.com:29443"
)

type Config struct {
	DBPath   string         `yaml:"db_path"`
	KIS      KISConfig      `yaml:"kis"`
	LS       LSConfig       `yaml:"ls"`
	Log      LogConfig      `yaml:"log"`
	Universe UniverseConfig `yaml:"universe"`
	Fetch    FetchConfig    `yaml:"fetch"`
	Strategy StrategyConfig `yaml:"strategy"`
}

type KISConfig struct {
	Env                string  `yaml:"env"`
	TokenCache         string  `yaml:"token_cache"`
	RequestsPerSecond  float64 `yaml:"requests_per_second"`
	BalancePollSeconds int     `yaml:"balance_poll_seconds"`
	AppKey             string  `yaml:"-"`
	AppSecret          string  `yaml:"-"`
	Account            string  `yaml:"-"` // "12345678-01". 비어 있으면 보유 종목 미연결
}

func (k KISConfig) BaseURL() string {
	if k.Env == "real" {
		return RealBaseURL
	}
	return DemoBaseURL
}

// EffectiveRPS 는 설정값이 0 이면 env 기본값(실전 15, 모의 1.5)을 돌려준다.
func (k KISConfig) EffectiveRPS() float64 {
	if k.RequestsPerSecond > 0 {
		return k.RequestsPerSecond
	}
	if k.Env == "real" {
		return 15
	}
	return 1.5
}

// LSConfig 는 LS증권 실시간 웹소켓 설정이다. 앱키가 없으면 TUI 는 뉴스·지수를 미연결로 표시한다.
type LSConfig struct {
	BaseURL    string `yaml:"base_url"`
	WSURL      string `yaml:"ws_url"`
	TokenCache string `yaml:"token_cache"`
	AppKey     string `yaml:"-"`
	AppSecret  string `yaml:"-"`
}

func (l LSConfig) HasAppKey() bool { return l.AppKey != "" && l.AppSecret != "" }

type LogConfig struct {
	File string `yaml:"file"`
}

type UniverseConfig struct {
	Markets      []string `yaml:"markets"`
	GroupCodes   []string `yaml:"group_codes"`
	ExcludeFlags []string `yaml:"exclude_flags"`
}

type FetchConfig struct {
	StartDate string `yaml:"start_date"`
}

type StrategyConfig struct {
	IndexMADays      int     `yaml:"index_ma_days"`
	MinTurnover      int64   `yaml:"min_turnover"`
	TurnoverMADays   int     `yaml:"turnover_ma_days"`
	TurnoverRatioMin float64 `yaml:"turnover_ratio_min"`
	CloseToHighMin   float64 `yaml:"close_to_high_min"`
	ChangeMin        float64 `yaml:"change_min"`
	ChangeMax        float64 `yaml:"change_max"`
	MAShortDays      int     `yaml:"ma_short_days"`
	MALongDays       int     `yaml:"ma_long_days"`
	NewHighDays      int     `yaml:"new_high_days"`
}

// Load 는 yaml 을 읽고 환경변수(KIS_APP_KEY, KIS_APP_SECRET, KIS_ACCOUNT, LS_APP_KEY, LS_APP_SECRET)를 덧붙인 뒤 검증한다.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg := &Config{DBPath: "data/market.db"}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.KIS.AppKey = os.Getenv("KIS_APP_KEY")
	cfg.KIS.AppSecret = os.Getenv("KIS_APP_SECRET")
	cfg.KIS.Account = os.Getenv("KIS_ACCOUNT")
	cfg.LS.AppKey = os.Getenv("LS_APP_KEY")
	cfg.LS.AppSecret = os.Getenv("LS_APP_SECRET")
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	var errs []error
	if c.KIS.Env != "demo" && c.KIS.Env != "real" {
		errs = append(errs, fmt.Errorf("kis.env must be demo or real, got %q", c.KIS.Env))
	}
	if c.KIS.TokenCache == "" {
		errs = append(errs, errors.New("kis.token_cache is required"))
	}
	if c.KIS.RequestsPerSecond < 0 {
		errs = append(errs, errors.New("kis.requests_per_second must be >= 0"))
	}
	if c.KIS.BalancePollSeconds <= 0 {
		errs = append(errs, errors.New("kis.balance_poll_seconds must be > 0"))
	}
	if c.LS.BaseURL == "" || c.LS.WSURL == "" || c.LS.TokenCache == "" {
		errs = append(errs, errors.New("ls.base_url, ls.ws_url, ls.token_cache are required"))
	}
	if c.Log.File == "" {
		errs = append(errs, errors.New("log.file is required"))
	}
	if len(c.Universe.Markets) == 0 {
		errs = append(errs, errors.New("universe.markets is required"))
	}
	for _, m := range c.Universe.Markets {
		if m != "kospi" && m != "kosdaq" {
			errs = append(errs, fmt.Errorf("universe.markets: unknown market %q", m))
		}
	}
	if _, err := time.Parse("2006-01-02", c.Fetch.StartDate); err != nil {
		errs = append(errs, fmt.Errorf("fetch.start_date must be YYYY-MM-DD: %w", err))
	}
	s := c.Strategy
	for name, v := range map[string]int{
		"index_ma_days": s.IndexMADays, "turnover_ma_days": s.TurnoverMADays,
		"ma_short_days": s.MAShortDays, "ma_long_days": s.MALongDays, "new_high_days": s.NewHighDays,
	} {
		if v <= 0 {
			errs = append(errs, fmt.Errorf("strategy.%s must be > 0", name))
		}
	}
	if s.MAShortDays >= s.MALongDays {
		errs = append(errs, errors.New("strategy.ma_short_days must be < ma_long_days"))
	}
	if s.ChangeMin >= s.ChangeMax {
		errs = append(errs, errors.New("strategy.change_min must be < change_max"))
	}
	for name, v := range map[string]float64{
		"turnover_ratio_min": s.TurnoverRatioMin, "close_to_high_min": s.CloseToHighMin,
		"change_min": s.ChangeMin,
	} {
		if v < 0 {
			errs = append(errs, fmt.Errorf("%s must be >= 0", name))
		}
	}
	if s.MinTurnover < 0 {
		errs = append(errs, errors.New("strategy.min_turnover must be >= 0"))
	}
	return errors.Join(errs...)
}

// RequireAppKey 는 한투 네트워크를 쓰는 서브커맨드가 시작 전에 호출한다.
func (c *Config) RequireAppKey() error {
	if c.KIS.AppKey == "" || c.KIS.AppSecret == "" {
		return errors.New("KIS_APP_KEY / KIS_APP_SECRET 환경변수가 없습니다 (.env 파일을 확인하세요)")
	}
	return nil
}
```

`internal/config/dotenv.go`:

```go
package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// LoadDotEnv 는 KEY=VALUE 파일을 읽어 아직 비어 있는 환경변수만 채운다. 파일이 없으면 nil.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if cur, set := os.LookupEnv(k); set && cur != "" {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return sc.Err()
}
```

- [ ] **Step 6: 테스트 통과 확인**

Run: `go test ./internal/config/ -v 2>&1 | tail -15`
Expected: 6개 테스트 모두 PASS

- [ ] **Step 7: 커밋 요청**

사용자에게 아래 명령 실행을 요청한다.

```bash
git add go.mod go.sum config.yaml internal/config
git commit -m "feat(config): 설정 파일과 .env 로딩"
```

---

### Task 2: TUI 모델 — 메시지 타입, 키 처리, 스크롤

**Files:**
- Create: `internal/tui/types.go`, `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Interfaces:**
- Produces: `tui.New() Model`, `Model`이 `tea.Model` 구현. 외부 메시지 타입 `NewsMsg`, `IndexMsg`, `WatchMsg`, `HoldingsMsg`, `LogMsg`와 행 타입 `WatchRow`, `HoldingRow`, `HoldingsSummary`, `LogLine`, `MarketFilter`. 2·3·7·8단계는 이 메시지들을 `(*tea.Program).Send`로 밀어 넣는다. 비공개 필드 `focus`, `menuCursor`, `active`, `cursor`, `offset`은 Task 3 View 와 같은 패키지 테스트가 읽는다.

- [ ] **Step 1: 타입 작성**

`internal/tui/types.go`:

```go
// Package tui 는 bubbletea 전체 화면 하나를 그린다. 계산은 하지 않고 메시지로 받은 값만 표시한다.
package tui

import "time"

// WatchRow 는 관심종목 패널 한 줄. 7단계에서 screener.WatchItem 을 여기로 옮긴다.
type WatchRow struct {
	Name, Market string
	PrevClose    int64
	MinClose     int64
	MinChangePct float64 // 0.031 = +3.1%
	MinVolume    int64
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

type WatchMsg struct {
	AsOf   string // "09-12"
	Rows   []WatchRow
	Filter MarketFilter
}

// HoldingsMsg 의 Connected 가 false 면 "미연결" 로 표시한다.
type HoldingsMsg struct {
	Connected bool
	Rows      []HoldingRow
	Summary   HoldingsSummary
}

// LogMsg 는 최신순으로 정렬된 전체 목록을 담는다 (증분 아님).
type LogMsg struct {
	Lines []LogLine
}
```

- [ ] **Step 2: 실패하는 테스트 작성**

`internal/tui/model_test.go`:

```go
package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func sized(t *testing.T) Model {
	t.Helper()
	m, _ := New().Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	return m.(Model)
}

func press(m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(k)
	return next.(Model), cmd
}

func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

var (
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyQ     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
)

func watchRows(n int) []WatchRow {
	rows := make([]WatchRow, n)
	for i := range rows {
		rows[i] = WatchRow{Name: "종목", Market: "코스피", PrevClose: 1000, MinClose: 1030, MinChangePct: 0.03, MinVolume: 100}
	}
	return rows
}

func TestInitialState(t *testing.T) {
	m := sized(t)
	if m.focus != focusMenu || m.active != panelWatch || m.menuCursor != 0 {
		t.Errorf("initial state: focus=%v active=%v menu=%d", m.focus, m.active, m.menuCursor)
	}
}

func TestTabTogglesFocus(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyTab)
	if m.focus != focusPanel {
		t.Fatal("tab should move focus to panel")
	}
	m, _ = press(m, keyTab)
	if m.focus != focusMenu {
		t.Fatal("tab should move focus back to menu")
	}
}

func TestMenuNavigationAndEnter(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyUp) // 위로 못 감
	if m.menuCursor != 0 {
		t.Errorf("menu cursor went above 0: %d", m.menuCursor)
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, keyDown)
	}
	if m.menuCursor != len(menuLabels)-1 {
		t.Errorf("menu cursor = %d, want %d", m.menuCursor, len(menuLabels)-1)
	}
	m, _ = press(m, keyEnter)
	if m.active != panelLog {
		t.Errorf("enter should activate log panel, got %v", m.active)
	}
	m, _ = press(m, keyUp)
	m, _ = press(m, keyEnter)
	if m.active != panelHoldings {
		t.Errorf("enter should activate holdings panel, got %v", m.active)
	}
}

func TestPanelCursorStaysInBounds(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{AsOf: "09-12", Rows: watchRows(3)})
	m, _ = press(m, keyTab)
	for i := 0; i < 5; i++ {
		m, _ = press(m, keyDown)
	}
	if m.cursor[panelWatch] != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor[panelWatch])
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, keyUp)
	}
	if m.cursor[panelWatch] != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor[panelWatch])
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := sized(t) // 높이 24 → 본문 18줄 → 표 행 14줄
	m = send(m, WatchMsg{AsOf: "09-12", Rows: watchRows(50)})
	m, _ = press(m, keyTab)
	for i := 0; i < 20; i++ {
		m, _ = press(m, keyDown)
	}
	if m.cursor[panelWatch] != 20 {
		t.Fatalf("cursor = %d", m.cursor[panelWatch])
	}
	if got, want := m.offset[panelWatch], 20-m.visibleRows()+1; got != want {
		t.Errorf("offset = %d, want %d (visibleRows=%d)", got, want, m.visibleRows())
	}
	for i := 0; i < 20; i++ {
		m, _ = press(m, keyUp)
	}
	if m.offset[panelWatch] != 0 {
		t.Errorf("offset after scrolling up = %d", m.offset[panelWatch])
	}
}

func TestNewDataResetsCursor(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{Rows: watchRows(10)})
	m, _ = press(m, keyTab)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyDown)
	m = send(m, WatchMsg{Rows: watchRows(1)})
	if m.cursor[panelWatch] != 0 || m.offset[panelWatch] != 0 {
		t.Errorf("cursor/offset not reset: %d/%d", m.cursor[panelWatch], m.offset[panelWatch])
	}
}

func TestQuit(t *testing.T) {
	m := sized(t)
	_, cmd := press(m, keyQ)
	if cmd == nil {
		t.Fatal("q should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should produce tea.QuitMsg")
	}
}

func TestTickUpdatesClockAndReschedules(t *testing.T) {
	m := sized(t)
	at := time.Date(2026, 9, 15, 14, 32, 10, 0, time.Local)
	next, cmd := m.Update(tickMsg(at))
	if !next.(Model).now.Equal(at) {
		t.Error("now not updated")
	}
	if cmd == nil {
		t.Error("tick should reschedule")
	}
}

func TestDataMessagesStored(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Source: "연합뉴스", Title: "제목"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	m = send(m, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004})
	m = send(m, HoldingsMsg{Connected: true, Rows: []HoldingRow{{Name: "삼성전자"}}})
	m = send(m, LogMsg{Lines: []LogLine{{Kind: "수집", Msg: "시작"}}})
	if !m.newsOK || m.news.Title != "제목" {
		t.Error("news not stored")
	}
	if !m.kospi.Connected || m.kospi.Value != 2712.4 || !m.kosdaq.Connected || m.kosdaq.ChangePct != -0.004 {
		t.Errorf("index not stored: %+v %+v", m.kospi, m.kosdaq)
	}
	if len(m.holdings.Rows) != 1 || len(m.logs) != 1 {
		t.Error("holdings/logs not stored")
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: New` 등)

- [ ] **Step 4: 모델 구현**

`internal/tui/model.go`:

```go
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type focus int

const (
	focusMenu focus = iota
	focusPanel
)

type panel int

const (
	panelWatch panel = iota
	panelHoldings
	panelLog
	panelCount
)

var menuLabels = [panelCount]string{"관심종목", "보유종목", "로그"}

// 고정 줄 수: 상단 괘선, 뉴스, 괘선, (본문), 괘선, 하단 정보, 괘선
const chromeLines = 6

// 패널 안에서 표 위에 쓰는 줄 수: 제목, 빈 줄, 헤더, 괘선
const panelHeaderLines = 4

type indexQuote struct {
	Value     float64
	ChangePct float64
	Connected bool
}

type tickMsg time.Time

type Model struct {
	width, height int
	now           time.Time

	focus      focus
	menuCursor int
	active     panel
	cursor     [panelCount]int
	offset     [panelCount]int

	news     NewsMsg
	newsOK   bool
	kospi    indexQuote
	kosdaq   indexQuote
	watch    WatchMsg
	holdings HoldingsMsg
	logs     []LogLine
}

func New() Model {
	return Model{now: time.Now()}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd { return tick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case NewsMsg:
		m.news, m.newsOK = msg, true
	case IndexMsg:
		q := indexQuote{Value: msg.Value, ChangePct: msg.ChangePct, Connected: true}
		if msg.Market == "kospi" {
			m.kospi = q
		} else {
			m.kosdaq = q
		}
	case WatchMsg:
		m.watch = msg
		m.cursor[panelWatch], m.offset[panelWatch] = 0, 0
	case HoldingsMsg:
		m.holdings = msg
		m.cursor[panelHoldings], m.offset[panelHoldings] = 0, 0
	case LogMsg:
		m.logs = msg.Lines
		m.cursor[panelLog], m.offset[panelLog] = 0, 0
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		if m.focus == focusMenu {
			m.focus = focusPanel
		} else {
			m.focus = focusMenu
		}
	case "up":
		if m.focus == focusMenu {
			m.menuCursor = max(0, m.menuCursor-1)
		} else {
			m.cursor[m.active] = max(0, m.cursor[m.active]-1)
		}
	case "down":
		if m.focus == focusMenu {
			m.menuCursor = min(int(panelCount)-1, m.menuCursor+1)
		} else {
			m.cursor[m.active] = min(max(0, m.rowCount(m.active)-1), m.cursor[m.active]+1)
		}
	case "enter":
		if m.focus == focusMenu {
			m.active = panel(m.menuCursor)
		}
	}
	m.clampScroll()
	return m, nil
}

func (m Model) rowCount(p panel) int {
	switch p {
	case panelWatch:
		return len(m.watch.Rows)
	case panelHoldings:
		return len(m.holdings.Rows)
	default:
		return len(m.logs)
	}
}

// visibleRows 는 표 본문에 그릴 수 있는 행 수. 터미널이 작아도 최소 1.
func (m Model) visibleRows() int {
	return max(1, m.height-chromeLines-panelHeaderLines)
}

// clampScroll 은 활성 패널의 커서가 화면 안에 오도록 offset 을 조정한다.
func (m *Model) clampScroll() {
	p := m.active
	vis := m.visibleRows()
	if m.cursor[p] < m.offset[p] {
		m.offset[p] = m.cursor[p]
	}
	if m.cursor[p] >= m.offset[p]+vis {
		m.offset[p] = m.cursor[p] - vis + 1
	}
	if m.offset[p] < 0 {
		m.offset[p] = 0
	}
}

// View 는 view.go 에 있다.
```

`View`가 아직 없으면 `tea.Model` 인터페이스가 안 맞아 컴파일이 안 되므로, 이 Task 에서는 `internal/tui/view.go`에 임시로 아래 한 줄짜리 파일을 둔다. Task 3 에서 통째로 교체한다.

```go
package tui

func (m Model) View() string { return "" }
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/tui/ -v 2>&1 | tail -15`
Expected: 9개 테스트 모두 PASS

- [ ] **Step 6: 커밋 요청**

```bash
git add internal/tui
git commit -m "feat(tui): 모델, 메시지 타입, 키 처리, 스크롤"
```

---

### Task 3: View — 레이아웃, 표 렌더링, 숫자 포맷

**Files:**
- Modify: `internal/tui/view.go` (Task 2 의 임시 파일을 통째로 교체)
- Test: `internal/tui/view_test.go`

**Interfaces:**
- Consumes: Task 2 의 `Model` 필드 전부, `menuLabels`, `chromeLines`, `visibleRows()`.
- Produces: `(Model).View() string`. 비공개 헬퍼 `fit`, `spread`, `comma`, `commaF`, `pct` 는 이 파일 안에서만 쓴다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/view_test.go`:

```go
package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestViewHasExactHeightAndWidth(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{AsOf: "09-12", Rows: watchRows(50), Filter: MarketFilter{Kospi: "진입가능", Kosdaq: "차단"}})
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("view has %d lines, want 24", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("line %d width %d > 100: %q", i, w, l)
		}
	}
}

func TestViewDisconnectedDefaults(t *testing.T) {
	v := sized(t).View()
	if strings.Count(v, "미연결") < 3 { // 뉴스, 코스피, 코스닥
		t.Errorf("expected 미연결 for news and both indexes:\n%s", v)
	}
	if !strings.Contains(v, "Tab 패널  q 종료") {
		t.Error("key hint missing")
	}
}

func TestViewHeaderAndFooter(t *testing.T) {
	m := sized(t)
	m = send(m, tickMsg(time.Date(2026, 9, 15, 14, 32, 10, 0, time.Local)))
	m = send(m, NewsMsg{Source: "연합뉴스", Title: "삼성전자, 3분기 파운드리 수주 확대 전망"})
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	m = send(m, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004})
	v := m.View()
	for _, want := range []string{"[연합뉴스] 삼성전자, 3분기 파운드리 수주 확대 전망", "14:32:10", "코스피 2,712.40 ▲+0.8%", "코스닥 782.15 ▼-0.4%"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewWatchPanel(t *testing.T) {
	m := sized(t)
	m = send(m, WatchMsg{AsOf: "09-12", Filter: MarketFilter{Kospi: "진입가능", Kosdaq: "차단"}, Rows: []WatchRow{
		{Name: "삼성전자", Market: "코스피", PrevClose: 71200, MinClose: 73400, MinChangePct: 0.031, MinVolume: 2140000},
	}})
	v := m.View()
	for _, want := range []string{"관심종목  (09-12 기준, 1개)", "코스피 진입가능 · 코스닥 차단", "종목명", "필요거래량", "삼성전자", "71,200", "73,400", "+3.1%", "2,140,000"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewHoldingsPanel(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyEnter)
	if !strings.Contains(m.View(), "보유종목  미연결") {
		t.Errorf("holdings should show 미연결 before data:\n%s", m.View())
	}
	m = send(m, HoldingsMsg{Connected: true})
	if !strings.Contains(m.View(), "보유 없음") {
		t.Errorf("empty holdings should show 보유 없음:\n%s", m.View())
	}
	m = send(m, HoldingsMsg{Connected: true,
		Summary: HoldingsSummary{Total: 12480000, PnL: 312000, PnLPct: 0.026, Cash: 7520000},
		Rows:    []HoldingRow{{Name: "삼성전자", Market: "코스피", Qty: 58, AvgPrice: 71200, Price: 72900, PnL: 98600, PnLPct: 0.024, HoldDays: 1}, {Name: "에코프로", Market: "코스닥", Qty: 40, AvgPrice: 98500, Price: 96100, PnL: -96000, PnLPct: -0.024}},
	})
	v := m.View()
	for _, want := range []string{"보유종목  (2종목)", "평가금액 12,480,000", "손익 +312,000 (+2.6%)", "현금 7,520,000", "+98,600", "+2.4%", "1일", "-96,000", "-2.4%"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewLogPanel(t *testing.T) {
	m := sized(t)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyDown)
	m, _ = press(m, keyEnter)
	m = send(m, LogMsg{Lines: []LogLine{{Time: time.Date(2026, 9, 15, 6, 12, 40, 0, time.Local), Kind: "수집", Msg: "일봉 2,391종목 완료"}}})
	v := m.View()
	for _, want := range []string{"로그", "최신순", "06:12:40", "[수집]", "일봉 2,391종목 완료"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestViewMenuCursorFollowsFocus(t *testing.T) {
	m := sized(t)
	if !strings.Contains(m.View(), "> 관심종목") {
		t.Error("menu cursor should be on 관심종목 when menu focused")
	}
	m, _ = press(m, keyTab)
	if strings.Contains(m.View(), "> 관심종목") {
		t.Error("menu cursor should disappear when panel focused")
	}
}

func TestViewTruncatesLongText(t *testing.T) {
	m := sized(t)
	m = send(m, NewsMsg{Source: "출처", Title: strings.Repeat("긴제목", 60)})
	for i, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("line %d width %d > 100", i, w)
		}
	}
}

func TestFormatHelpers(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1,000", 2140000: "2,140,000", -96000: "-96,000"}
	for n, want := range cases {
		if got := comma(n); got != want {
			t.Errorf("comma(%d) = %q, want %q", n, got, want)
		}
	}
	if got := commaF(2712.4); got != "2,712.40" {
		t.Errorf("commaF = %q", got)
	}
	if got := pct(0.031); got != "+3.1%" {
		t.Errorf("pct = %q", got)
	}
	if got := pct(-0.024); got != "-2.4%" {
		t.Errorf("pct = %q", got)
	}
	if got := fit("삼성전자", 10, false); lipgloss.Width(got) != 10 {
		t.Errorf("fit pad width = %d", lipgloss.Width(got))
	}
	if got := fit("삼성전자", 5, false); lipgloss.Width(got) != 5 {
		t.Errorf("fit truncate width = %d: %q", lipgloss.Width(got), got)
	}
	if got := fit("12", 6, true); got != "    12" {
		t.Errorf("fit right = %q", got)
	}
	if got := spread("L", "R", 10); got != "L"+strings.Repeat(" ", 8)+"R" {
		t.Errorf("spread = %q", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: comma` 등)

- [ ] **Step 3: View 구현**

`internal/tui/view.go` (Task 2 의 임시 파일을 이 내용으로 교체):

```go
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const menuWidth = 14 // "  > 관심종목  "

// 색·굵기 스타일은 이 단계에서 쓰지 않는다. ANSI 코드가 끼면 문구 포함 테스트가 깨지고, 폭 계산도 복잡해진다.
const keyHint = "Tab 패널  q 종료"

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	w := m.width
	rule := strings.Repeat("─", w)
	header := spread(m.headerLeft(), m.now.Format("15:04:05")+" ", w)
	footer := spread(m.footerLeft(), keyHint+" ", w)
	lines := make([]string, 0, m.height)
	lines = append(lines, rule, header, rule)
	lines = append(lines, m.bodyLines()...)
	lines = append(lines, rule, footer, rule)
	return strings.Join(lines, "\n")
}

func (m Model) headerLeft() string {
	if !m.newsOK {
		return " 미연결"
	}
	return fmt.Sprintf(" [%s] %s", m.news.Source, m.news.Title)
}

func (m Model) footerLeft() string {
	return " 코스피 " + fmtIndex(m.kospi) + "   코스닥 " + fmtIndex(m.kosdaq)
}

func fmtIndex(q indexQuote) string {
	if !q.Connected {
		return "미연결"
	}
	arrow := "▲"
	if q.ChangePct < 0 {
		arrow = "▼"
	}
	return commaF(q.Value) + " " + arrow + pct(q.ChangePct)
}

// bodyLines 는 왼쪽 메뉴와 가운데 패널을 한 줄씩 붙여 본문 줄들을 만든다.
func (m Model) bodyLines() []string {
	rows := max(1, m.height-chromeLines)
	panelW := m.width - menuWidth - 1
	menu := m.menuLines(rows)
	panel := m.panelLines(panelW, rows)
	out := make([]string, rows)
	for i := range out {
		out[i] = fit(menu[i], menuWidth, false) + "│" + fit(panel[i], panelW, false)
	}
	return out
}

func (m Model) menuLines(rows int) []string {
	out := make([]string, rows)
	for i, label := range menuLabels {
		if i+1 >= rows {
			break
		}
		marker := "  "
		if m.focus == focusMenu && m.menuCursor == i {
			marker = "> "
		}
		out[i+1] = "  " + marker + label
	}
	return out
}

func (m Model) panelLines(w, rows int) []string {
	var title string
	var cols []column
	var body [][]string
	switch m.active {
	case panelWatch:
		title, cols, body = m.watchPanel(w)
	case panelHoldings:
		title, cols, body = m.holdingsPanel(w)
	default:
		title, cols, body = m.logPanel(w)
	}
	out := []string{title, ""}
	out = append(out, renderTable(cols, body, m.cursor[m.active], m.offset[m.active], m.visibleRows(), m.focus == focusPanel)...)
	for len(out) < rows {
		out = append(out, "")
	}
	return out[:rows]
}

type column struct {
	title string
	width int
	right bool
}

// renderTable 은 헤더, 괘선, 보이는 행들을 돌려준다. body 가 nil 이면 rows 자리에 빈 줄만 남긴다.
func renderTable(cols []column, body [][]string, cursor, offset, visible int, focused bool) []string {
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = fit(c.title, c.width, c.right)
	}
	total := 2 // 커서 표시 폭
	for _, c := range cols {
		total += c.width + 1
	}
	out := []string{"  " + strings.Join(head, " "), strings.Repeat("─", total)}
	end := min(len(body), offset+visible)
	for i := offset; i < end; i++ {
		marker := "  "
		if focused && i == cursor {
			marker = "> "
		}
		cells := make([]string, len(cols))
		for j, c := range cols {
			cells[j] = fit(body[i][j], c.width, c.right)
		}
		out = append(out, marker+strings.Join(cells, " "))
	}
	return out
}

func (m Model) watchPanel(w int) (string, []column, [][]string) {
	f := m.watch.Filter
	if f.Kospi == "" {
		f.Kospi, f.Kosdaq = "알 수 없음", "알 수 없음"
	}
	asOf := m.watch.AsOf
	if asOf == "" {
		asOf = "-"
	}
	title := spread(fmt.Sprintf(" 관심종목  (%s 기준, %d개)", asOf, len(m.watch.Rows)),
		fmt.Sprintf("코스피 %s · 코스닥 %s ", f.Kospi, f.Kosdaq), w)
	cols := []column{{"종목명", 16, false}, {"시장", 6, false}, {"전일종가", 10, true}, {"필요종가", 10, true}, {"필요상승", 8, true}, {"필요거래량", 12, true}}
	body := make([][]string, len(m.watch.Rows))
	for i, r := range m.watch.Rows {
		body[i] = []string{r.Name, r.Market, comma(r.PrevClose), comma(r.MinClose), pct(r.MinChangePct), comma(r.MinVolume)}
	}
	return title, cols, body
}

func (m Model) holdingsPanel(w int) (string, []column, [][]string) {
	cols := []column{{"종목명", 16, false}, {"시장", 6, false}, {"수량", 6, true}, {"매입가", 10, true}, {"현재가", 10, true}, {"손익", 12, true}, {"수익률", 8, true}, {"보유일", 6, true}}
	h := m.holdings
	if !h.Connected {
		return " 보유종목  미연결", cols, nil
	}
	if len(h.Rows) == 0 {
		return " 보유종목  보유 없음", cols, nil
	}
	s := h.Summary
	title := spread(fmt.Sprintf(" 보유종목  (%d종목)", len(h.Rows)),
		fmt.Sprintf("평가금액 %s   손익 %s (%s)   현금 %s ", comma(s.Total), signed(s.PnL), pct(s.PnLPct), comma(s.Cash)), w)
	body := make([][]string, len(h.Rows))
	for i, r := range h.Rows {
		days := "-"
		if r.HoldDays > 0 {
			days = strconv.Itoa(r.HoldDays) + "일"
		}
		body[i] = []string{r.Name, r.Market, comma(r.Qty), comma(r.AvgPrice), comma(r.Price), signed(r.PnL), pct(r.PnLPct), days}
	}
	return title, cols, body
}

func (m Model) logPanel(w int) (string, []column, [][]string) {
	msgW := max(10, w-20) // 커서 2 + 시각 8 + 종류 6 + 구분 공백 3, 여유 1
	cols := []column{{"시각", 8, false}, {"종류", 6, false}, {"메시지", msgW, false}}
	body := make([][]string, len(m.logs))
	for i, l := range m.logs {
		body[i] = []string{l.Time.Format("15:04:05"), "[" + l.Kind + "]", l.Msg}
	}
	return spread(" 로그", "최신순 ", w), cols, body
}

// fit 은 s 를 폭 w 에 맞춘다. 길면 "…" 로 자르고 짧으면 공백을 채운다 (right 면 오른쪽 정렬).
func fit(s string, w int, right bool) string {
	s = ansi.Truncate(s, w, "…")
	pad := strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
	if right {
		return pad + s
	}
	return s + pad
}

// spread 는 left 를 왼쪽, right 를 오른쪽 끝에 놓고 사이를 공백으로 채운 폭 w 의 한 줄을 만든다.
func spread(left, right string, w int) string {
	rw := lipgloss.Width(right)
	left = ansi.Truncate(left, max(0, w-rw-1), "…")
	gap := max(1, w-lipgloss.Width(left)-rw)
	return left + strings.Repeat(" ", gap) + right
}

func comma(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func commaF(v float64) string {
	s := fmt.Sprintf("%.2f", v) // "2712.40"
	dot := strings.LastIndex(s, ".")
	whole, _ := strconv.ParseInt(s[:dot], 10, 64)
	return comma(whole) + s[dot:]
}

func signed(n int64) string {
	if n > 0 {
		return "+" + comma(n)
	}
	return comma(n)
}

func pct(v float64) string {
	return fmt.Sprintf("%+.1f%%", v*100)
}
```

`go.mod`에 `github.com/charmbracelet/x/ansi`가 직접 의존성으로 없으면 `go mod tidy`로 올린다.

- [ ] **Step 4: 테스트 통과 확인**

Run: `go mod tidy && go test ./internal/tui/ -v 2>&1 | tail -25`
Expected: Task 2 의 9개 + 이 Task 의 9개 테스트 모두 PASS. `go.mod`에 `github.com/charmbracelet/x/ansi` require 추가됨.

- [ ] **Step 5: 커밋 요청**

```bash
git add go.mod go.sum internal/tui
git commit -m "feat(tui): 레이아웃과 패널 렌더링"
```

---

### Task 4: 가짜 데이터, Run, 진입점, 수동 확인

**Files:**
- Create: `internal/tui/fake.go`, `internal/tui/run.go`, `cmd/trader/main.go`

**Interfaces:**
- Consumes: `config.LoadDotEnv`, `config.Load`, Task 2·3 의 `Model`과 메시지 타입.
- Produces: `tui.Run(ctx context.Context, cfg *config.Config) error`. 6단계가 `main.go`에 서브커맨드를 더하고, 2·3·7·8단계가 `Run` 안에서 가짜 메시지를 실데이터 소스로 바꾼다.

- [ ] **Step 1: 가짜 데이터 작성**

`internal/tui/fake.go`:

```go
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeMessages 는 1단계 화면 확인용 가짜 데이터다.
// 2단계(뉴스·지수), 3단계(로그), 7단계(관심종목), 8단계(보유종목)에서 실데이터로 바꾸며 해당 항목을 지우고, 다 지워지면 이 파일을 삭제한다.
func fakeMessages(now time.Time) []tea.Msg {
	return []tea.Msg{
		NewsMsg{Source: "연합뉴스", Title: "삼성전자, 3분기 파운드리 수주 확대 전망"},
		IndexMsg{Market: "kospi", Value: 2712.40, ChangePct: 0.008},
		IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004},
		WatchMsg{AsOf: "09-12", Filter: MarketFilter{Kospi: "진입가능", Kosdaq: "차단"}, Rows: []WatchRow{
			{Name: "삼성전자", Market: "코스피", PrevClose: 71200, MinClose: 73400, MinChangePct: 0.031, MinVolume: 2140000},
			{Name: "SK하이닉스", Market: "코스피", PrevClose: 182000, MinClose: 188100, MinChangePct: 0.034, MinVolume: 620000},
			{Name: "에코프로", Market: "코스닥", PrevClose: 98500, MinClose: 102300, MinChangePct: 0.039, MinVolume: 910000},
			{Name: "현대차", Market: "코스피", PrevClose: 245000, MinClose: 255100, MinChangePct: 0.041, MinVolume: 380000},
			{Name: "LG에너지솔루션", Market: "코스피", PrevClose: 398000, MinClose: 412500, MinChangePct: 0.036, MinVolume: 150000},
			{Name: "셀트리온", Market: "코스피", PrevClose: 178500, MinClose: 184200, MinChangePct: 0.032, MinVolume: 420000},
			{Name: "알테오젠", Market: "코스닥", PrevClose: 312000, MinClose: 325000, MinChangePct: 0.042, MinVolume: 210000},
		}},
		HoldingsMsg{Connected: true, Summary: HoldingsSummary{Total: 12480000, PnL: 312000, PnLPct: 0.026, Cash: 7520000}, Rows: []HoldingRow{
			{Name: "삼성전자", Market: "코스피", Qty: 58, AvgPrice: 71200, Price: 72900, PnL: 98600, PnLPct: 0.024, HoldDays: 1},
			{Name: "에코프로", Market: "코스닥", Qty: 40, AvgPrice: 98500, Price: 96100, PnL: -96000, PnLPct: -0.024, HoldDays: 1},
			{Name: "현대차", Market: "코스피", Qty: 16, AvgPrice: 245000, Price: 264300, PnL: 308800, PnLPct: 0.079, HoldDays: 2},
		}},
		LogMsg{Lines: []LogLine{
			{Time: now.Add(-1 * time.Minute), Kind: "연결", Msg: "LS 웹소켓 재연결 성공"},
			{Time: now.Add(-2 * time.Minute), Kind: "연결", Msg: "LS 웹소켓 연결 끊김, 5초 후 재시도"},
			{Time: now.Add(-5 * time.Hour), Kind: "지수", Msg: "코스피 20일선 위 · 코스닥 20일선 아래 → 코스닥 진입 차단"},
			{Time: now.Add(-8 * time.Hour), Kind: "수집", Msg: "일봉 2,391종목 완료, 실패 3 (000660 타임아웃)"},
			{Time: now.Add(-8*time.Hour - 3*time.Minute), Kind: "수집", Msg: "유니버스 갱신 2,391종목 (+2 −5)"},
			{Time: now.Add(-8*time.Hour - 4*time.Minute), Kind: "수집", Msg: "시작"},
		}},
	}
}
```

- [ ] **Step 2: Run 작성**

`internal/tui/run.go`:

```go
package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
)

// Run 은 전체 화면 TUI 를 띄우고 종료될 때까지 막는다. cfg 는 2단계(LS)·8단계(잔고)부터 쓴다.
func Run(ctx context.Context, cfg *config.Config) error {
	_ = cfg
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithContext(ctx))
	go func() {
		for _, msg := range fakeMessages(time.Now()) {
			p.Send(msg)
		}
	}()
	_, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil // Ctrl+C 로 컨텍스트가 취소된 정상 종료
	}
	return err
}
```

- [ ] **Step 3: main.go 작성**

`cmd/trader/main.go`:

```go
// trader 는 종가 베팅 운영 도구의 CLI 진입점이다.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

const usage = `사용법:
  trader                 TUI
  trader universe        (6단계 계획에서 구현)
  trader fetch           (6단계 계획에서 구현)
  trader watch           (7단계 계획에서 구현)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf(".env: %w", err)
	}
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return tui.Run(ctx, cfg)
	}
	switch args[0] {
	case "universe", "fetch", "watch":
		return fmt.Errorf("%s 는 아직 구현되지 않았습니다", args[0])
	default:
		fmt.Print(usage)
		return fmt.Errorf("알 수 없는 명령 %q", args[0])
	}
}
```

- [ ] **Step 4: 빌드와 전체 테스트**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... 2>&1 | tail -5`
Expected: gofmt 출력 없음, vet 통과, `config`·`tui` 패키지 `ok`

- [ ] **Step 5: 잘못된 명령과 미구현 명령 확인**

Run: `go run ./cmd/trader nope; echo "exit=$?"; go run ./cmd/trader fetch; echo "exit=$?"`
Expected: 첫 번째는 사용법 출력 후 `오류: 알 수 없는 명령 "nope"` exit=1, 두 번째는 `오류: fetch 는 아직 구현되지 않았습니다` exit=1

- [ ] **Step 6: 화면 수동 확인 (사용자)**

Claude 는 터미널 화면을 볼 수 없다. 사용자에게 아래를 실행하고 결과를 말로 알려 달라고 요청한다. 터미널 폭 100 이상, 높이 24 이상.

```bash
go run ./cmd/trader
```

확인 항목:
1. 전체 화면으로 뜨고, 맨 위 줄에 `[연합뉴스] 삼성전자, …`, 오른쪽 끝에 현재 시각이 초 단위로 움직인다.
2. 맨 아래 줄에 `코스피 2,712.40 ▲+0.8%   코스닥 782.15 ▼-0.4%`, 오른쪽 끝에 `Tab 패널  q 종료`.
3. 왼쪽에 `> 관심종목 / 보유종목 / 로그`, 가운데에 관심종목 표 7행. 한글 열이 어긋나지 않는다.
4. `↓` `Enter` 로 보유종목(3행 + 제목 줄 요약), 한 번 더 `↓` `Enter` 로 로그(6행) 패널로 바뀐다.
5. `Tab` 후 `↑↓` 로 표 안의 `>` 커서가 움직이고, 메뉴의 `>` 는 사라진다. 어느 패널이 활성인지는 가운데 제목 줄로 안다.
6. 터미널 크기를 바꾸면 화면이 따라온다. `q` 로 종료하면 원래 터미널 내용이 복원된다.

어긋나는 항목이 있으면 사용자 설명을 바탕으로 `view.go`를 고치고 `go test ./internal/tui/`를 다시 돌린 뒤 재확인한다.

- [ ] **Step 7: 커밋 요청**

```bash
git add cmd/trader internal/tui
git commit -m "feat(tui): 가짜 데이터로 전체 화면 뼈대, trader 진입점"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- `go run ./cmd/trader` 가 설계 10.1절 레이아웃을 가짜 데이터로 띄우고, 위 수동 확인 6개 항목을 사용자가 확인함
- 다음 단계(2단계 LS 웹소켓)는 `config.LSConfig`, `tui.NewsMsg`, `tui.IndexMsg`, `tui.Run` 을 그대로 소비하고 `fake.go`의 뉴스·지수 항목을 지운다
