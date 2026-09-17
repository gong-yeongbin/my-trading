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

// KISConfig 는 한투 Open API 설정. 시세(일봉·지수·마스터) 수집은 항상 실전 서버·실전 키(Market)를
// 쓴다 — 모의 서버는 시세 조회가 느리고 값은 모의·실전이 같기 때문이다. TradeEnv 는 매매·잔고(8단계)에만 적용된다.
type KISConfig struct {
	TradeEnv           string  `yaml:"trade_env"`           // demo | real — 매매·잔고에만 적용
	TokenCache         string  `yaml:"token_cache"`         // 실전(시세) 토큰 캐시
	TradeTokenCache    string  `yaml:"trade_token_cache"`   // 매매용 토큰 캐시 (비어 있으면 token_cache + ".trade")
	RequestsPerSecond  float64 `yaml:"requests_per_second"` // 0 이면 15
	BalancePollSeconds int     `yaml:"balance_poll_seconds"`
	Market             Creds   `yaml:"-"` // 실전 키. KIS_REAL_APP_KEY / KIS_REAL_APP_SECRET / KIS_REAL_ACCOUNT
	Trade              Creds   `yaml:"-"` // TradeEnv 에 따른 키. demo 면 KIS_DEMO_*(구 KIS_APP_KEY 등 fallback), real 이면 Market 과 같음
}

// Creds 는 앱키·시크릿·계좌 한 벌.
type Creds struct{ AppKey, AppSecret, Account string }

// MarketBaseURL 은 시세 수집(일봉·지수·마스터)이 쓰는 서버. 항상 실전.
func (k KISConfig) MarketBaseURL() string { return RealBaseURL }

// TradeBaseURL 은 매매·잔고가 쓰는 서버. TradeEnv 로 고른다.
func (k KISConfig) TradeBaseURL() string {
	if k.TradeEnv == "real" {
		return RealBaseURL
	}
	return DemoBaseURL
}

// MarketRPS 는 시세 수집 호출 제한. RequestsPerSecond 가 0 이면 15(실전 기본값).
func (k KISConfig) MarketRPS() float64 {
	if k.RequestsPerSecond > 0 {
		return k.RequestsPerSecond
	}
	return 15
}

// TradeRPS 는 매매·잔고 호출 제한. TradeEnv 기본값(실전 15, 모의 1.5).
func (k KISConfig) TradeRPS() float64 {
	if k.TradeEnv == "real" {
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
	DailyAt   string `yaml:"daily_at"` // HH:MM KST. TUI 가 매일 이 시각에 전날 일봉을 받는다. 비어 있으면 04:00
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

// legacyKISEnv 는 구 config.yaml 의 kis.env 값을 읽기 위한 임시 구조. trade_env 가 없을 때만 마이그레이션에 쓴다.
type legacyKISEnv struct {
	KIS struct {
		Env string `yaml:"env"`
	} `yaml:"kis"`
}

// Load 는 yaml 을 읽고 환경변수(시세는 KIS_REAL_*, 매매는 TradeEnv 에 따라 KIS_DEMO_*/KIS_REAL_*
// 또는 KIS_APP_KEY/KIS_APP_SECRET/KIS_ACCOUNT fallback, LS_APP_KEY, LS_APP_SECRET)를 덧붙인 뒤 검증한다.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg := &Config{DBPath: "data/market.db"}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if cfg.Fetch.DailyAt == "" {
		cfg.Fetch.DailyAt = "04:00"
	}
	if cfg.KIS.TradeEnv == "" {
		var legacy legacyKISEnv
		if err := yaml.Unmarshal(b, &legacy); err == nil {
			cfg.KIS.TradeEnv = legacy.KIS.Env
		}
	}
	if cfg.KIS.TradeEnv == "" {
		cfg.KIS.TradeEnv = "demo"
	}
	if cfg.KIS.TradeTokenCache == "" && cfg.KIS.TokenCache != "" {
		cfg.KIS.TradeTokenCache = cfg.KIS.TokenCache + ".trade"
	}

	cfg.KIS.Market.AppKey = os.Getenv("KIS_REAL_APP_KEY")
	cfg.KIS.Market.AppSecret = os.Getenv("KIS_REAL_APP_SECRET")
	cfg.KIS.Market.Account = os.Getenv("KIS_REAL_ACCOUNT")

	if cfg.KIS.TradeEnv == "real" {
		cfg.KIS.Trade = cfg.KIS.Market
	} else {
		cfg.KIS.Trade.AppKey = envOr("KIS_DEMO_APP_KEY", "KIS_APP_KEY")
		cfg.KIS.Trade.AppSecret = envOr("KIS_DEMO_APP_SECRET", "KIS_APP_SECRET")
		cfg.KIS.Trade.Account = envOr("KIS_DEMO_ACCOUNT", "KIS_ACCOUNT")
	}

	cfg.LS.AppKey = os.Getenv("LS_APP_KEY")
	cfg.LS.AppSecret = os.Getenv("LS_APP_SECRET")
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	var errs []error
	if c.KIS.TradeEnv != "demo" && c.KIS.TradeEnv != "real" {
		errs = append(errs, fmt.Errorf("kis.trade_env must be demo or real, got %q", c.KIS.TradeEnv))
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
	if _, err := time.Parse("15:04", c.Fetch.DailyAt); err != nil || len(c.Fetch.DailyAt) != 5 {
		errs = append(errs, fmt.Errorf("fetch.daily_at must be HH:MM: %q", c.Fetch.DailyAt))
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

// RequireMarketKey 는 시세(일봉·지수·마스터)를 쓰는 서브커맨드·자동 수집이 시작 전에 호출한다.
// 시세 수집은 항상 실전 키가 필요하다 (모의 fallback 없음).
func (c *Config) RequireMarketKey() error {
	if c.KIS.Market.AppKey == "" || c.KIS.Market.AppSecret == "" {
		return errors.New("실전 앱키(KIS_REAL_APP_KEY / KIS_REAL_APP_SECRET)가 없습니다 — 시세 수집은 실전 키가 필요합니다. .env 또는 TUI 설정 메뉴에서 입력")
	}
	return nil
}

// RequireTradeKey 는 매매·잔고(8단계)를 쓰는 기능이 시작 전에 호출한다.
func (c *Config) RequireTradeKey() error {
	if c.KIS.Trade.AppKey == "" || c.KIS.Trade.AppSecret == "" {
		prefix := "KIS_DEMO_"
		if c.KIS.TradeEnv == "real" {
			prefix = "KIS_REAL_"
		}
		return fmt.Errorf("%sAPP_KEY / %sAPP_SECRET 환경변수가 없습니다 (.env 또는 TUI 설정 메뉴)", prefix, prefix)
	}
	return nil
}

// envOr 는 첫 번째 환경변수가 비어 있으면 두 번째(구 이름)를 쓴다.
func envOr(name, legacy string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return os.Getenv(legacy)
}
