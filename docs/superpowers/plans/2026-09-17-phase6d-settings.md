# 6.7단계: TUI 설정 메뉴 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 왼쪽 메뉴에 `설정`이 생기고, 거기서 한투 서버(모의/실전) 전환, 한투·LS 앱키·시크릿, 계좌번호, 자동 수집 시각, 수집 시작일을 보고 고칠 수 있다. 저장하면 `.env`와 `config.yaml`에 쓰이고(다른 줄·주석 유지) "재시작하면 적용됩니다"가 보인다. 모의·실전 키를 둘 다 보관하고 서버 전환만으로 고른다.

**Architecture:** `config`가 `KIS_DEMO_*`/`KIS_REAL_*` 환경변수를 `kis.env`에 따라 고른다(기존 `KIS_APP_KEY`는 fallback). 새 패키지 `settings`가 필드 정의·표시 마스킹·검증·`.env`/`config.yaml` 읽기·쓰기(줄 단위 편집, yaml.Node로 주석 보존)를 맡는다. TUI 는 네 번째 패널 `설정`에서 목록·편집(`bubbles/textinput`)만 하고 저장은 주입된 `saver`를 부른다.

**Tech Stack:** `github.com/charmbracelet/bubbles/textinput` (설계 3절에 이미 허용), `gopkg.in/yaml.v3` Node API. 그 외 새 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (3절 의존성, 5절 설정, 10.1절 메뉴, 11절). Task 4 에서 갱신.

**선행:** 6.5단계 완료.

## Global Constraints

- 환경변수: `KIS_DEMO_APP_KEY`, `KIS_DEMO_APP_SECRET`, `KIS_DEMO_ACCOUNT`, `KIS_REAL_APP_KEY`, `KIS_REAL_APP_SECRET`, `KIS_REAL_ACCOUNT`, `LS_APP_KEY`, `LS_APP_SECRET`. `config.Load`는 `kis.env`에 맞는 접두사를 쓰고, 없으면 기존 `KIS_APP_KEY`/`KIS_APP_SECRET`/`KIS_ACCOUNT`로 fallback (기존 `.env` 호환).
- `.env` 쓰기: 기존 줄 순서·주석·다른 키 유지. 해당 키 줄이 있으면 값만 교체, 없으면 끝에 추가. 파일 0600. 값에 따옴표 없이 씀.
- `config.yaml` 쓰기: `yaml.v3` `yaml.Node`로 읽어 `kis.env`, `fetch.daily_at`, `fetch.start_date`만 바꾸고 다시 씀. 주석은 보존된다(들여쓰기 2).
- 표시: 앱키·시크릿은 앞 4자 + `****`, 비어 있으면 `(없음)`. 시크릿 입력은 `*`로 가림.
- 검증: 서버 `demo|real`; 시각 `HH:MM`; 시작일 `YYYY-MM-DD`; 계좌 `\d{8}-\d{2}` 또는 빈 값; 키·시크릿은 공백 없는 문자열 또는 빈 값.
- 키: 설정 패널에서 `↑↓` 항목 이동, `Enter` 편집 시작(서버 항목은 토글 후 즉시 저장), 편집 중 `Enter` 저장·`Esc` 취소, 그 외 키는 입력창으로. 편집 중엔 `q`·`Tab`도 글자다.
- 저장 뒤 화면 문구: `저장됨 · 재시작하면 적용됩니다`. 실패하면 `저장 실패: …`. 실행 중인 클라이언트는 갈아끼우지 않는다.
- TUI 는 로직을 갖지 않는다: 마스킹·검증·파일 쓰기는 `settings`. 색 금지. 폭은 `lipgloss.Width`/`ansi.Truncate`.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- 프로젝트 루트의 실제 `.env`는 읽지도 쓰지도 않는다 — 테스트는 전부 임시 파일. `go run ./cmd/trader`(인자 없음) 실행 금지. `gofmt -l .` 비어야 함.

---

## 파일 구조

```
internal/config/config.go         env 별 키 선택 (수정)
internal/config/config_test.go    (수정)
internal/settings/settings.go     Field, Values, Labels, IsSecret, Display, Validate
internal/settings/files.go        Load, Save (.env 줄 편집, yaml.Node 편집)
internal/settings/settings_test.go
internal/settings/files_test.go
internal/tui/types.go             SettingsMsg (수정)
internal/tui/model.go             panelSettings, 편집 상태, 키 처리 (수정)
internal/tui/settings.go          설정 패널 렌더·편집 (신규)
internal/tui/settings_test.go
internal/tui/run.go               settings.Load → SettingsMsg, saver 주입 (수정)
internal/tui/model_test.go        메뉴 4개 반영 (수정)
config.yaml, .env.example         (수정/신규)
```

---

### Task 1: config — 서버별 앱키 선택

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Create: `.env.example`

**Interfaces:**
- Produces: `Load` 가 `kis.env`에 따라 `KIS_DEMO_*`/`KIS_REAL_*`를 읽고, 없으면 `KIS_APP_KEY`/`KIS_APP_SECRET`/`KIS_ACCOUNT` fallback. 시그니처 변화 없음.

- [ ] **Step 1: 테스트 추가**

`internal/config/config_test.go` 끝에:

```go
func TestLoadPicksKeysByEnv(t *testing.T) {
	for _, v := range []string{"KIS_APP_KEY", "KIS_APP_SECRET", "KIS_ACCOUNT"} {
		t.Setenv(v, "")
	}
	t.Setenv("KIS_DEMO_APP_KEY", "dk")
	t.Setenv("KIS_DEMO_APP_SECRET", "ds")
	t.Setenv("KIS_DEMO_ACCOUNT", "11111111-01")
	t.Setenv("KIS_REAL_APP_KEY", "rk")
	t.Setenv("KIS_REAL_APP_SECRET", "rs")
	t.Setenv("KIS_REAL_ACCOUNT", "22222222-01")

	cfg, err := Load(writeTemp(t, "c.yaml", goodYAML)) // env: demo
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.AppKey != "dk" || cfg.KIS.AppSecret != "ds" || cfg.KIS.Account != "11111111-01" {
		t.Errorf("demo keys: %+v", cfg.KIS)
	}
	real := strings.Replace(goodYAML, "env: demo", "env: real", 1)
	cfg, err = Load(writeTemp(t, "c.yaml", real))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.AppKey != "rk" || cfg.KIS.AppSecret != "rs" || cfg.KIS.Account != "22222222-01" {
		t.Errorf("real keys: %+v", cfg.KIS)
	}
}

func TestLoadFallsBackToLegacyKeys(t *testing.T) {
	for _, v := range []string{"KIS_DEMO_APP_KEY", "KIS_DEMO_APP_SECRET", "KIS_DEMO_ACCOUNT", "KIS_REAL_APP_KEY", "KIS_REAL_APP_SECRET", "KIS_REAL_ACCOUNT"} {
		t.Setenv(v, "")
	}
	t.Setenv("KIS_APP_KEY", "lk")
	t.Setenv("KIS_APP_SECRET", "ls")
	t.Setenv("KIS_ACCOUNT", "33333333-01")
	cfg, err := Load(writeTemp(t, "c.yaml", goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KIS.AppKey != "lk" || cfg.KIS.AppSecret != "ls" || cfg.KIS.Account != "33333333-01" {
		t.Errorf("legacy fallback: %+v", cfg.KIS)
	}
}
```

`TestLoadGood` 는 `KIS_APP_KEY` 등을 설정하므로 fallback 으로 그대로 통과해야 한다. 그 테스트 앞부분에 `KIS_DEMO_*` 를 빈 값으로 `t.Setenv` 해 두면 다른 테스트와 독립적이다.

- [ ] **Step 2: 구현**

`internal/config/config.go` 의 `Load` 에서 환경변수 읽는 부분을 이렇게 바꾼다:

```go
	prefix := "KIS_DEMO_"
	if cfg.KIS.Env == "real" {
		prefix = "KIS_REAL_"
	}
	cfg.KIS.AppKey = envOr(prefix+"APP_KEY", "KIS_APP_KEY")
	cfg.KIS.AppSecret = envOr(prefix+"APP_SECRET", "KIS_APP_SECRET")
	cfg.KIS.Account = envOr(prefix+"ACCOUNT", "KIS_ACCOUNT")
	cfg.LS.AppKey = os.Getenv("LS_APP_KEY")
	cfg.LS.AppSecret = os.Getenv("LS_APP_SECRET")
```

파일 끝에:

```go
// envOr 는 첫 번째 환경변수가 비어 있으면 두 번째(구 이름)를 쓴다.
func envOr(name, legacy string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return os.Getenv(legacy)
}
```

`Load` 의 doc 주석과 `RequireAppKey` 의 오류 문구를 새 이름으로 갱신: `"KIS_DEMO_APP_KEY / KIS_DEMO_APP_SECRET (또는 KIS_REAL_*) 환경변수가 없습니다 (.env 또는 TUI 설정 메뉴)"` — `Env` 에 따라 접두사를 문구에 넣는다.

`.env.example` (루트, git 에 포함):

```
# 한투 Open API — 모의투자
KIS_DEMO_APP_KEY=
KIS_DEMO_APP_SECRET=
KIS_DEMO_ACCOUNT=
# 한투 Open API — 실전투자
KIS_REAL_APP_KEY=
KIS_REAL_APP_SECRET=
KIS_REAL_ACCOUNT=
# LS증권 Open API (실시간 뉴스·지수)
LS_APP_KEY=
LS_APP_SECRET=
```

`config.yaml` 의 `kis:` 주석 줄을 `# 앱키·계좌는 .env 의 KIS_DEMO_* / KIS_REAL_* (env 에 따라 선택). TUI 설정 메뉴에서 입력 가능` 로 바꾼다.

- [ ] **Step 3: 확인**

Run: `gofmt -l . ; go vet ./internal/config/ && go test ./internal/config/ -count=1 -v 2>&1 | tail -6`
Expected: 새 테스트 2개 포함 전부 PASS

- [ ] **Step 4: 커밋 요청**

```bash
git add internal/config config.yaml .env.example
git commit -m "feat(config): 모의·실전 앱키를 KIS_DEMO_*/KIS_REAL_* 로 분리, 기존 이름 fallback"
```

---

### Task 2: settings 패키지 — 필드·검증·파일 읽기쓰기

**Files:**
- Create: `internal/settings/settings.go`, `internal/settings/files.go`, `internal/settings/settings_test.go`, `internal/settings/files_test.go`

**Interfaces:**
- Produces: `settings.Field`(상수 `KISEnv, KISDemoKey, KISDemoSecret, KISDemoAccount, KISRealKey, KISRealSecret, KISRealAccount, LSKey, LSSecret, DailyAt, StartDate, FieldCount`), `settings.Labels [FieldCount]string`, `settings.Values` (필드마다 string), `(Values).Get(Field) string`, `(*Values).Set(Field, string)`, `settings.IsSecret(Field) bool`, `settings.Display(Field, string) string`, `settings.Validate(Field, string) error`, `settings.Load(envPath, yamlPath string) (Values, error)`, `settings.Save(envPath, yamlPath string, v Values) error`. Task 3 이 전부 쓴다.

- [ ] **Step 1: settings.go 테스트**

`internal/settings/settings_test.go`:

```go
package settings

import "testing"

func TestDisplayMasksSecrets(t *testing.T) {
	if got := Display(KISDemoKey, "PSabcdefghijklmnop"); got != "PSab****" {
		t.Errorf("key mask = %q", got)
	}
	if got := Display(KISDemoSecret, "abc"); got != "****" {
		t.Errorf("short secret = %q", got)
	}
	if got := Display(KISDemoKey, ""); got != "(없음)" {
		t.Errorf("empty = %q", got)
	}
	if got := Display(KISEnv, "demo"); got != "demo" {
		t.Errorf("plain = %q", got)
	}
	if got := Display(DailyAt, ""); got != "(없음)" {
		t.Errorf("empty plain = %q", got)
	}
}

func TestIsSecret(t *testing.T) {
	for _, f := range []Field{KISDemoKey, KISDemoSecret, KISRealKey, KISRealSecret, LSKey, LSSecret} {
		if !IsSecret(f) {
			t.Errorf("%s should be secret", Labels[f])
		}
	}
	for _, f := range []Field{KISEnv, KISDemoAccount, KISRealAccount, DailyAt, StartDate} {
		if IsSecret(f) {
			t.Errorf("%s should not be secret", Labels[f])
		}
	}
}

func TestValidate(t *testing.T) {
	ok := map[Field][]string{
		KISEnv:         {"demo", "real"},
		DailyAt:        {"04:00", "23:59"},
		StartDate:      {"2025-09-01"},
		KISDemoAccount: {"", "12345678-01"},
		KISDemoKey:     {"", "PSabc123"},
	}
	bad := map[Field][]string{
		KISEnv:         {"", "paper"},
		DailyAt:        {"4:00", "24:00", ""},
		StartDate:      {"2025/09/01", ""},
		KISDemoAccount: {"1234567801", "12345678-1"},
		KISDemoKey:     {"has space"},
	}
	for f, vals := range ok {
		for _, v := range vals {
			if err := Validate(f, v); err != nil {
				t.Errorf("%s %q should be valid: %v", Labels[f], v, err)
			}
		}
	}
	for f, vals := range bad {
		for _, v := range vals {
			if err := Validate(f, v); err == nil {
				t.Errorf("%s %q should be invalid", Labels[f], v)
			}
		}
	}
}

func TestGetSetRoundTrip(t *testing.T) {
	var v Values
	for f := Field(0); f < FieldCount; f++ {
		v.Set(f, "x"+Labels[f])
	}
	for f := Field(0); f < FieldCount; f++ {
		if v.Get(f) != "x"+Labels[f] {
			t.Errorf("field %d round trip", f)
		}
	}
}
```

- [ ] **Step 2: settings.go 구현**

`internal/settings/settings.go`:

```go
// Package settings 는 TUI 설정 메뉴가 다루는 값의 정의·표시·검증과 .env / config.yaml 읽기쓰기를 맡는다.
package settings

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Field int

const (
	KISEnv Field = iota
	KISDemoKey
	KISDemoSecret
	KISDemoAccount
	KISRealKey
	KISRealSecret
	KISRealAccount
	LSKey
	LSSecret
	DailyAt
	StartDate
	FieldCount
)

var Labels = [FieldCount]string{
	"한투 서버", "한투 모의 앱키", "한투 모의 시크릿", "한투 모의 계좌",
	"한투 실전 앱키", "한투 실전 시크릿", "한투 실전 계좌",
	"LS 앱키", "LS 시크릿", "자동 수집 시각", "수집 시작일",
}

// Values 는 설정 메뉴의 모든 값. 문자열 그대로 보관한다.
type Values struct {
	KISEnv, KISDemoKey, KISDemoSecret, KISDemoAccount string
	KISRealKey, KISRealSecret, KISRealAccount         string
	LSKey, LSSecret, DailyAt, StartDate               string
}

func (v Values) Get(f Field) string {
	switch f {
	case KISEnv:
		return v.KISEnv
	case KISDemoKey:
		return v.KISDemoKey
	case KISDemoSecret:
		return v.KISDemoSecret
	case KISDemoAccount:
		return v.KISDemoAccount
	case KISRealKey:
		return v.KISRealKey
	case KISRealSecret:
		return v.KISRealSecret
	case KISRealAccount:
		return v.KISRealAccount
	case LSKey:
		return v.LSKey
	case LSSecret:
		return v.LSSecret
	case DailyAt:
		return v.DailyAt
	case StartDate:
		return v.StartDate
	}
	return ""
}

func (v *Values) Set(f Field, s string) {
	switch f {
	case KISEnv:
		v.KISEnv = s
	case KISDemoKey:
		v.KISDemoKey = s
	case KISDemoSecret:
		v.KISDemoSecret = s
	case KISDemoAccount:
		v.KISDemoAccount = s
	case KISRealKey:
		v.KISRealKey = s
	case KISRealSecret:
		v.KISRealSecret = s
	case KISRealAccount:
		v.KISRealAccount = s
	case LSKey:
		v.LSKey = s
	case LSSecret:
		v.LSSecret = s
	case DailyAt:
		v.DailyAt = s
	case StartDate:
		v.StartDate = s
	}
}

// IsSecret 은 화면에서 가려야 하는 필드.
func IsSecret(f Field) bool {
	switch f {
	case KISDemoKey, KISDemoSecret, KISRealKey, KISRealSecret, LSKey, LSSecret:
		return true
	}
	return false
}

// Display 는 목록에 보일 문자열. 비밀은 앞 4자만 남기고 가린다.
func Display(f Field, v string) string {
	if v == "" {
		return "(없음)"
	}
	if !IsSecret(f) {
		return v
	}
	r := []rune(v)
	if len(r) <= 4 {
		return "****"
	}
	return string(r[:4]) + "****"
}

var (
	accountRe = regexp.MustCompile(`^\d{8}-\d{2}$`)
	tokenRe   = regexp.MustCompile(`^\S+$`)
)

// Validate 는 저장 전 값 검사.
func Validate(f Field, v string) error {
	switch f {
	case KISEnv:
		if v != "demo" && v != "real" {
			return errors.New("demo 또는 real")
		}
	case DailyAt:
		if _, err := time.Parse("15:04", v); err != nil || len(v) != 5 {
			return errors.New("HH:MM 형식")
		}
	case StartDate:
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return errors.New("YYYY-MM-DD 형식")
		}
	case KISDemoAccount, KISRealAccount:
		if v != "" && !accountRe.MatchString(v) {
			return errors.New("12345678-01 형식")
		}
	default:
		if v != "" && !tokenRe.MatchString(strings.TrimSpace(v)) {
			return fmt.Errorf("공백 없는 문자열")
		}
	}
	return nil
}
```

Run: `go test ./internal/settings/ -run 'TestDisplay|TestIsSecret|TestValidate|TestGetSet' -count=1` → ok.

- [ ] **Step 3: files.go 테스트**

`internal/settings/files_test.go`:

```go
package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleEnv = `# 한투
KIS_DEMO_APP_KEY=oldkey
KIS_DEMO_APP_SECRET="oldsecret"
OTHER=keep me
# LS
LS_APP_KEY=lk
`

const sampleYAML = `db_path: data/market.db

kis:
  env: demo                    # demo / real
  token_cache: data/token.json

fetch:
  start_date: "2025-09-01"     # 시작일
  daily_at: "04:00"

strategy:
  index_ma_days: 20
`

func write(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadReadsBothFiles(t *testing.T) {
	v, err := Load(write(t, ".env", sampleEnv), write(t, "config.yaml", sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if v.KISDemoKey != "oldkey" || v.KISDemoSecret != "oldsecret" || v.LSKey != "lk" || v.KISRealKey != "" {
		t.Errorf("env values: %+v", v)
	}
	if v.KISEnv != "demo" || v.DailyAt != "04:00" || v.StartDate != "2025-09-01" {
		t.Errorf("yaml values: %+v", v)
	}
}

func TestLoadMissingEnvFileIsEmpty(t *testing.T) {
	v, err := Load(filepath.Join(t.TempDir(), "none.env"), write(t, "config.yaml", sampleYAML))
	if err != nil || v.KISDemoKey != "" || v.KISEnv != "demo" {
		t.Errorf("missing .env: %+v %v", v, err)
	}
}

func TestSavePreservesOtherLinesAndComments(t *testing.T) {
	envPath := write(t, ".env", sampleEnv)
	yamlPath := write(t, "config.yaml", sampleYAML)
	v, _ := Load(envPath, yamlPath)
	v.KISDemoKey = "newkey"
	v.KISRealKey = "rk"
	v.KISEnv = "real"
	v.DailyAt = "05:30"
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}

	env, _ := os.ReadFile(envPath)
	s := string(env)
	for _, want := range []string{"# 한투\n", "KIS_DEMO_APP_KEY=newkey\n", "KIS_DEMO_APP_SECRET=oldsecret\n", "OTHER=keep me\n", "# LS\n", "LS_APP_KEY=lk\n", "KIS_REAL_APP_KEY=rk\n"} {
		if !strings.Contains(s, want) {
			t.Errorf(".env missing %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "KIS_DEMO_APP_KEY=") != 1 {
		t.Errorf("key line duplicated:\n%s", s)
	}
	if strings.Index(s, "# 한투") > strings.Index(s, "KIS_DEMO_APP_KEY=") {
		t.Errorf("line order changed:\n%s", s)
	}
	if info, _ := os.Stat(envPath); info.Mode().Perm() != 0o600 {
		t.Errorf(".env perm = %o", info.Mode().Perm())
	}

	y, _ := os.ReadFile(yamlPath)
	ys := string(y)
	for _, want := range []string{"env: real", "# demo / real", "daily_at: \"05:30\"", "start_date: \"2025-09-01\"", "# 시작일", "index_ma_days: 20", "token_cache: data/token.json"} {
		if !strings.Contains(ys, want) {
			t.Errorf("yaml missing %q:\n%s", want, ys)
		}
	}
	// 다시 읽어도 같은 값
	v2, err := Load(envPath, yamlPath)
	if err != nil || v2.KISDemoKey != "newkey" || v2.KISEnv != "real" || v2.DailyAt != "05:30" || v2.KISRealKey != "rk" {
		t.Errorf("reload: %+v %v", v2, err)
	}
}

func TestSaveCreatesEnvWhenMissing(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	yamlPath := write(t, "config.yaml", sampleYAML)
	v := Values{KISEnv: "demo", DailyAt: "04:00", StartDate: "2025-09-01", LSKey: "k"}
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(envPath)
	if !strings.Contains(string(env), "LS_APP_KEY=k\n") {
		t.Errorf("created .env: %s", env)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	envPath := write(t, ".env", sampleEnv)
	yamlPath := write(t, "config.yaml", sampleYAML)
	v, _ := Load(envPath, yamlPath)
	v.DailyAt = "bad"
	if err := Save(envPath, yamlPath, v); err == nil {
		t.Error("invalid value should not be saved")
	}
	env, _ := os.ReadFile(envPath)
	if !strings.Contains(string(env), "KIS_DEMO_APP_KEY=oldkey") {
		t.Error("files must be untouched on validation failure")
	}
}
```

- [ ] **Step 4: files.go 구현**

`internal/settings/files.go`:

```go
package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// envKeys 는 .env 에 쓰는 필드와 변수 이름.
var envKeys = map[Field]string{
	KISDemoKey: "KIS_DEMO_APP_KEY", KISDemoSecret: "KIS_DEMO_APP_SECRET", KISDemoAccount: "KIS_DEMO_ACCOUNT",
	KISRealKey: "KIS_REAL_APP_KEY", KISRealSecret: "KIS_REAL_APP_SECRET", KISRealAccount: "KIS_REAL_ACCOUNT",
	LSKey: "LS_APP_KEY", LSSecret: "LS_APP_SECRET",
}

// envOrder 는 새로 추가할 때의 순서.
var envOrder = []Field{KISDemoKey, KISDemoSecret, KISDemoAccount, KISRealKey, KISRealSecret, KISRealAccount, LSKey, LSSecret}

// yamlPaths 는 config.yaml 에 쓰는 필드와 경로.
var yamlPaths = map[Field][]string{
	KISEnv: {"kis", "env"}, DailyAt: {"fetch", "daily_at"}, StartDate: {"fetch", "start_date"},
}

// Load 는 .env(없으면 빈 값)와 config.yaml 에서 값을 읽는다.
func Load(envPath, yamlPath string) (Values, error) {
	var v Values
	lines, err := readLines(envPath)
	if err != nil {
		return v, err
	}
	for f, key := range envKeys {
		if val, ok := envValue(lines, key); ok {
			v.Set(f, val)
		}
	}
	root, err := readYAML(yamlPath)
	if err != nil {
		return v, err
	}
	for f, path := range yamlPaths {
		if n := findNode(root, path); n != nil {
			v.Set(f, n.Value)
		}
	}
	return v, nil
}

// Save 는 모든 필드를 검증한 뒤 .env 와 config.yaml 을 고쳐 쓴다. 검증 실패면 아무것도 쓰지 않는다.
func Save(envPath, yamlPath string, v Values) error {
	for f := Field(0); f < FieldCount; f++ {
		if err := Validate(f, v.Get(f)); err != nil {
			return fmt.Errorf("%s: %w", Labels[f], err)
		}
	}
	lines, err := readLines(envPath)
	if err != nil {
		return err
	}
	for _, f := range envOrder {
		lines = setEnv(lines, envKeys[f], v.Get(f))
	}
	root, err := readYAML(yamlPath)
	if err != nil {
		return err
	}
	for f, path := range yamlPaths {
		setNode(root, path, v.Get(f))
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return fmt.Errorf("settings: yaml: %w", err)
	}
	enc.Close()
	if err := os.WriteFile(envPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("settings: .env: %w", err)
	}
	if err := os.WriteFile(yamlPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("settings: config.yaml: %w", err)
	}
	return nil
}

func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

// envValue 는 KEY=VALUE 줄(주석 제외)의 값. 따옴표는 벗긴다.
func envValue(lines []string, key string) (string, bool) {
	for _, line := range lines {
		k, val, ok := splitEnv(line)
		if ok && k == key {
			return val, true
		}
	}
	return "", false
}

func splitEnv(line string) (string, string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	k, v, ok := strings.Cut(t, "=")
	if !ok {
		return "", "", false
	}
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		v = v[1 : len(v)-1]
	}
	return k, v, true
}

// setEnv 는 key 줄이 있으면 그 자리에 KEY=value 로 바꾸고, 없으면 끝에 붙인다.
func setEnv(lines []string, key, value string) []string {
	for i, line := range lines {
		if k, _, ok := splitEnv(line); ok && k == key {
			lines[i] = key + "=" + value
			return lines
		}
	}
	return append(lines, key+"="+value)
}

func readYAML(path string) (*yaml.Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("settings: yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, errors.New("settings: config.yaml 이 비어 있음")
	}
	return &doc, nil
}

// findNode 는 문서 루트에서 path(예: kis, env)의 값 노드.
func findNode(doc *yaml.Node, path []string) *yaml.Node {
	n := doc.Content[0]
	for _, key := range path {
		if n.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				next = n.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		n = next
	}
	return n
}

// setNode 는 path 의 값을 바꾼다. 중간 맵이나 키가 없으면 만든다. 문자열은 따옴표 스타일을 유지한다.
func setNode(doc *yaml.Node, path []string, value string) {
	n := doc.Content[0]
	for i, key := range path {
		var next *yaml.Node
		for j := 0; j+1 < len(n.Content); j += 2 {
			if n.Content[j].Value == key {
				next = n.Content[j+1]
				break
			}
		}
		if next == nil {
			k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			if i == len(path)-1 {
				next = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle}
			} else {
				next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			}
			n.Content = append(n.Content, k, next)
		}
		n = next
	}
	n.Kind, n.Tag, n.Value = yaml.ScalarNode, "!!str", value
}
```

- [ ] **Step 5: 확인**

Run: `gofmt -l . ; go vet ./internal/settings/ && go test ./internal/settings/ -count=1 -v 2>&1 | tail -12`
Expected: 9개 테스트 PASS. `TestSavePreservesOtherLinesAndComments` 가 yaml 주석 보존을 검증한다 — 실패하면 `yaml.v3` Node 인코딩 결과를 보고서에 붙이고 기대값을 바꾸지 말 것.

- [ ] **Step 6: 커밋 요청**

```bash
git add internal/settings
git commit -m "feat(settings): 설정 값 정의·검증, .env·config.yaml 읽기쓰기"
```

---

### Task 3: TUI 설정 패널

**Files:**
- Modify: `internal/tui/types.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/run.go`, `internal/tui/model_test.go`
- Create: `internal/tui/settings.go`, `internal/tui/settings_test.go`

**Interfaces:**
- Consumes: `settings.*` (Task 2), `bubbles/textinput`.
- Produces: `tui.SettingsMsg{Values settings.Values; Err error}`, 메뉴 4번째 `설정`(`panelSettings`), `Model.saver func(settings.Values) error` (`New()` 는 nil → 저장 시 `저장 실패: 저장기 없음`), `WithSaver(fn) Model` 옵션 없이 `run.go` 가 `m := New(); m.saver = …` 로 넣는다.

- [ ] **Step 1: 의존성**

```bash
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 2: 실패하는 테스트 작성**

`internal/tui/model_test.go` 의 `TestMenuNavigationAndEnter` 는 `len(menuLabels)-1` 로 마지막 항목을 검사하므로 그대로 두되, `m.active != panelLog` 검사를 `panelSettings` 로 바꾼다 (마지막 메뉴가 설정이 됨). 그 아래 `m, _ = press(m, keyUp); m, _ = press(m, keyEnter); if m.active != panelHoldings` 는 `panelLog` 로 바꾼다.

`internal/tui/settings_test.go`:

```go
package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/settings"
)

var keyEsc = tea.KeyMsg{Type: tea.KeyEsc}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// openSettings 는 메뉴에서 설정을 고르고 패널로 포커스를 옮긴다.
func openSettings(t *testing.T, saved *settings.Values, saveErr error) Model {
	t.Helper()
	m := sized(t)
	m.saver = func(v settings.Values) error {
		if saveErr != nil {
			return saveErr
		}
		*saved = v
		return nil
	}
	m = send(m, SettingsMsg{Values: settings.Values{KISEnv: "demo", KISDemoKey: "PSdemo1234", DailyAt: "04:00", StartDate: "2025-09-01"}})
	for i := 0; i < int(panelSettings); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	m, _ = press(m, keyTab)
	return m
}

func TestSettingsPanelShowsMaskedValues(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	v := m.View()
	for _, want := range []string{"설정", "한투 서버", "demo", "한투 모의 앱키", "PSde****", "자동 수집 시각", "04:00", "(없음)"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "PSdemo1234") {
		t.Error("secret must be masked")
	}
}

func TestSettingsToggleEnvSavesImmediately(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	m, _ = press(m, keyEnter) // 커서 0 = 한투 서버
	if saved.KISEnv != "real" || m.settings.KISEnv != "real" {
		t.Errorf("toggle should save real: saved=%+v", saved)
	}
	if !strings.Contains(m.View(), "저장됨 · 재시작하면 적용됩니다") {
		t.Errorf("save message missing:\n%s", m.View())
	}
	m, _ = press(m, keyEnter)
	if saved.KISEnv != "demo" {
		t.Errorf("toggle back: %+v", saved)
	}
}

func TestSettingsEditSaveAndCancel(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	// 자동 수집 시각으로 이동
	for i := 0; i < int(settings.DailyAt); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	if !m.editing {
		t.Fatal("enter should start editing")
	}
	// 편집 중엔 q 가 글자
	m = typeText(m, "q")
	if m.input.Value() != "04:00q" {
		t.Errorf("q should be typed, got %q", m.input.Value())
	}
	m, _ = press(m, keyEsc)
	if m.editing || m.settings.DailyAt != "04:00" {
		t.Errorf("esc should cancel: editing=%v value=%q", m.editing, m.settings.DailyAt)
	}
	// 다시 편집해 저장
	m, _ = press(m, keyEnter)
	m.input.SetValue("")
	m = typeText(m, "05:30")
	m, _ = press(m, keyEnter)
	if m.editing || m.settings.DailyAt != "05:30" || saved.DailyAt != "05:30" {
		t.Errorf("enter should save: editing=%v value=%q saved=%+v", m.editing, m.settings.DailyAt, saved)
	}
	if !strings.Contains(m.View(), "저장됨") {
		t.Errorf("save message missing:\n%s", m.View())
	}
}

func TestSettingsRejectsInvalidAndStaysEditing(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	for i := 0; i < int(settings.DailyAt); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	m.input.SetValue("")
	m = typeText(m, "5:30")
	m, _ = press(m, keyEnter)
	if !m.editing || saved.DailyAt != "" {
		t.Errorf("invalid value must not save: editing=%v saved=%+v", m.editing, saved)
	}
	if !strings.Contains(m.View(), "HH:MM") {
		t.Errorf("validation message missing:\n%s", m.View())
	}
}

func TestSettingsSecretInputIsMasked(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, nil)
	m, _ = press(m, keyDown) // 한투 모의 앱키
	m, _ = press(m, keyEnter)
	m.input.SetValue("")
	m = typeText(m, "PSnew")
	if strings.Contains(m.View(), "PSnew") {
		t.Errorf("secret input must be masked while typing:\n%s", m.View())
	}
	m, _ = press(m, keyEnter)
	if saved.KISDemoKey != "PSnew" {
		t.Errorf("saved = %+v", saved)
	}
}

func TestSettingsSaveErrorShown(t *testing.T) {
	var saved settings.Values
	m := openSettings(t, &saved, errors.New("disk full"))
	m, _ = press(m, keyEnter) // 서버 토글 → 저장 실패
	if !strings.Contains(m.View(), "저장 실패: disk full") {
		t.Errorf("error message missing:\n%s", m.View())
	}
}

func TestSettingsLoadErrorShown(t *testing.T) {
	m := sized(t)
	m = send(m, SettingsMsg{Err: errors.New("no yaml")})
	for i := 0; i < int(panelSettings); i++ {
		m, _ = press(m, keyDown)
	}
	m, _ = press(m, keyEnter)
	if !strings.Contains(m.View(), "설정 읽기 실패: no yaml") {
		t.Errorf("load error missing:\n%s", m.View())
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: panelSettings`, `SettingsMsg`, `m.saver`)

- [ ] **Step 4: types.go / model.go**

`types.go` 의 `FetchStatusMsg` 앞에 (import 에 `settings` 추가):

```go
// SettingsMsg 는 시작 시 읽은 설정 값. Err 가 있으면 패널에 오류를 보인다.
type SettingsMsg struct {
	Values settings.Values
	Err    error
}
```

`model.go`:
- `panelLog` 뒤에 `panelSettings` 추가 (`panelCount` 앞). `menuLabels = [panelCount]string{"관심종목", "보유종목", "로그", "설정"}`.
- `Model` 에 필드: `settings settings.Values; settingsErr error; settingsMsg string; editing bool; input textinput.Model; saver func(settings.Values) error`.
- `New()`: `ti := textinput.New(); ti.Prompt = ""; ti.CharLimit = 128; ti.Width = 40; return Model{now: time.Now(), input: ti}`.
- `Update` 맨 앞: 편집 중이면 키 처리를 설정 편집으로 보낸다.

```go
	if m.editing {
		if k, ok := msg.(tea.KeyMsg); ok {
			return m.handleEditKey(k)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
```
(단, `tea.WindowSizeMsg`·`tickMsg` 는 편집 중에도 처리해야 하므로 위 블록은 `switch` 안에서 `case tea.KeyMsg:` 를 대체하는 형태로 넣는다: `case tea.KeyMsg: if m.editing { return m.handleEditKey(msg) }; return m.handleKey(msg)`, 그리고 `default:` 에 `if m.editing { m.input, cmd = m.input.Update(msg); return m, cmd }`.)
- `Update` 에 `case SettingsMsg: m.settings, m.settingsErr = msg.Values, msg.Err`.
- `rowCount(panelSettings)` = `int(settings.FieldCount)`.
- `handleKey` 의 `case "enter"`: 패널 포커스이고 `m.active == panelSettings` 이면 `return m.settingsEnter()` (Step 5).

- [ ] **Step 5: settings.go (tui)**

`internal/tui/settings.go`:

```go
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/settings"
)

const savedMsg = "저장됨 · 재시작하면 적용됩니다"

// settingsEnter 는 설정 패널에서 Enter: 서버 항목은 토글 후 저장, 나머지는 편집 시작.
func (m Model) settingsEnter() (tea.Model, tea.Cmd) {
	f := settings.Field(m.cursor[panelSettings])
	if f == settings.KISEnv {
		v := m.settings
		if v.KISEnv == "real" {
			v.KISEnv = "demo"
		} else {
			v.KISEnv = "real"
		}
		m.save(v)
		return m, nil
	}
	m.editing = true
	m.settingsMsg = ""
	m.input.SetValue(m.settings.Get(f))
	if settings.IsSecret(f) {
		m.input.EchoMode = textinput.EchoPassword
		m.input.EchoCharacter = '*'
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.CursorEnd()
	return m, m.input.Focus()
}

// handleEditKey 는 편집 중 키. Enter 저장, Esc 취소, 나머지는 입력창으로.
func (m Model) handleEditKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.editing = false
		m.input.Blur()
		return m, nil
	case "enter":
		f := settings.Field(m.cursor[panelSettings])
		val := m.input.Value()
		if err := settings.Validate(f, val); err != nil {
			m.settingsMsg = "오류: " + err.Error()
			return m, nil
		}
		v := m.settings
		v.Set(f, val)
		m.editing = false
		m.input.Blur()
		m.save(v)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

// save 는 saver 로 저장하고 결과 문구를 남긴다. 성공하면 화면 값도 갱신.
func (m *Model) save(v settings.Values) {
	if m.saver == nil {
		m.settingsMsg = "저장 실패: 저장기 없음"
		return
	}
	if err := m.saver(v); err != nil {
		m.settingsMsg = "저장 실패: " + err.Error()
		return
	}
	m.settings = v
	m.settingsMsg = savedMsg
}

// settingsPanel 은 설정 목록. 편집 중인 항목의 값 칸에는 입력창이 들어간다.
func (m Model) settingsPanel(w int) (string, []column, [][]string) {
	right := "Enter 편집/토글 · Esc 취소 "
	title := spread(" 설정", right, w)
	if m.settingsErr != nil {
		title = spread(" 설정  읽기 실패: "+m.settingsErr.Error(), right, w)
	} else if m.settingsMsg != "" {
		title = spread(" 설정  "+m.settingsMsg, right, w)
	}
	cols := []column{{"항목", 16, false}, {"값", max(10, w-2-16-1), false}}
	body := make([][]string, settings.FieldCount)
	for f := settings.Field(0); f < settings.FieldCount; f++ {
		val := settings.Display(f, m.settings.Get(f))
		if m.editing && int(f) == m.cursor[panelSettings] {
			val = m.input.View()
		}
		body[f] = []string{settings.Labels[f], val}
	}
	return title, cols, body
}

var _ = fmt.Sprintf
```
(마지막 줄 `var _ = fmt.Sprintf` 는 넣지 않는다 — `fmt` 를 안 쓰면 import 도 빼라.)

`view.go` 의 `panelLines` switch 에 `case panelSettings: title, cols, body = m.settingsPanel(w)` 추가. 설정 읽기 실패 문구는 테스트가 `설정 읽기 실패: no yaml` 을 찾으므로 제목 형식을 ` 설정 읽기 실패: …` 로 맞춘다 (위 코드의 `" 설정  읽기 실패: "` 를 `" 설정 읽기 실패: "` 로).

- [ ] **Step 6: run.go**

`Run` 에서 `p := tea.NewProgram(New(), …)` 를:

```go
	m := New()
	m.saver = func(v settings.Values) error { return settings.Save(".env", "config.yaml", v) }
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	go func() {
		v, err := settings.Load(".env", "config.yaml")
		p.Send(SettingsMsg{Values: v, Err: err})
	}()
```
(import 에 `settings` 추가. `.env`·`config.yaml` 경로는 `main.go` 가 쓰는 상대 경로와 같다.)

- [ ] **Step 7: 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... -count=1 -race 2>&1 | tail -10`
Expected: 전부 ok. `go.mod` 에 `github.com/charmbracelet/bubbles` 직접 의존성.

- [ ] **Step 8: 커밋 요청**

```bash
git add go.mod go.sum internal/tui
git commit -m "feat(tui): 설정 메뉴 — 서버 전환, 앱키·계좌·수집 시각 편집, .env/config.yaml 저장"
```

---

### Task 4: 스펙 갱신과 수동 확인

- [ ] **Step 1: 스펙 갱신**

3절: 의존성에 `bubbles` 가 이미 있음 — `internal/settings/` 디렉터리 줄 추가. 5절 `kis:` 주석을 `KIS_DEMO_*`/`KIS_REAL_*` 로. 10.1절 메뉴 행을 "관심종목 / 보유종목 / 로그 / 설정" 으로 하고 패널 표에 설정 행 추가 (항목·값 목록, Enter 편집, 시크릿 마스킹, 저장 시 `.env`·`config.yaml` 갱신, 재시작 필요). 11절에 "설정 저장 실패는 패널에 표시, TUI 는 계속" 추가.

- [ ] **Step 2: 수동 확인 (사용자)**

1. `.env` 를 새 이름으로 옮기기: 기존 `KIS_APP_KEY=…` 줄을 `KIS_DEMO_APP_KEY=…` 로 (또는 그대로 두면 fallback 으로 동작).
2. `go run ./cmd/trader` → 메뉴 `설정` → 값이 마스킹되어 보임 → 서버 항목에서 Enter 로 `real` 토글 → `저장됨 · 재시작하면 적용됩니다` → `q` → `cat config.yaml | grep env` 에 `env: real`, 주석 유지 확인 → 다시 토글해 `demo` 로.
3. 실전 키 발급 후 `한투 실전 앱키`·`시크릿`·`실전 계좌` 입력 → 서버 `real` → 재시작 → 로그 패널에 `자동 수집 시작 env=real`.

- [ ] **Step 3: 커밋 요청**

```bash
git add docs
git commit -m "docs: 설정 메뉴, KIS_DEMO_*/KIS_REAL_* 반영"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `gofmt -l .` 비어 있음
- 설정 패널에서 서버 토글·값 편집·저장이 되고 파일의 다른 줄·주석이 유지됨
- 기존 `.env` (`KIS_APP_KEY`) 그대로도 동작함
