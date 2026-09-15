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
