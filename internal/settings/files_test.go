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
  trade_env: demo                    # demo / real
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
	for _, want := range []string{
		"trade_env: real                    # demo / real",
		"daily_at: \"05:30\"",
		"start_date: \"2025-09-01\"", "# 시작일",
		"index_ma_days: 20", "token_cache: data/token.json",
		"token_cache: data/token.json\n\nfetch:",
	} {
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
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf(".env perm = %o", info.Mode().Perm())
	}
}

func TestSaveAtomicLeavesNoTmp(t *testing.T) {
	envPath := write(t, ".env", sampleEnv)
	yamlPath := write(t, "config.yaml", sampleYAML)
	v, _ := Load(envPath, yamlPath)
	v.KISDemoKey = "anotherkey"
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Dir(envPath), filepath.Dir(yamlPath)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Errorf("leftover tmp file in %s: %s", dir, e.Name())
			}
		}
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

func TestLoadShowsLegacyKeysAsDemo(t *testing.T) {
	v, err := Load(write(t, ".env", "KIS_APP_KEY=legacykey\nKIS_ACCOUNT=12345678-01\n"), write(t, "config.yaml", sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if v.KISDemoKey != "legacykey" || v.KISDemoAccount != "12345678-01" || v.KISDemoSecret != "" {
		t.Errorf("legacy fallback: %+v", v)
	}
	// 새 이름이 있으면 그것이 우선
	v, _ = Load(write(t, ".env", "KIS_APP_KEY=legacykey\nKIS_DEMO_APP_KEY=newkey\n"), write(t, "config.yaml", sampleYAML))
	if v.KISDemoKey != "newkey" {
		t.Errorf("new name should win: %+v", v)
	}
}

func TestSaveDoesNotShadowLegacyKeys(t *testing.T) {
	envPath := write(t, ".env", "KIS_APP_KEY=legacykey\nKIS_APP_SECRET=legacysecret\nOTHER=x\n")
	yamlPath := write(t, "config.yaml", sampleYAML)
	v, _ := Load(envPath, yamlPath)
	if v.KISDemoKey != "legacykey" {
		t.Fatalf("load legacy: %+v", v)
	}
	// 아무것도 바꾸지 않고 저장: 빈 새 이름 줄이 생기면 안 되고, 구 이름 값은 그대로
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(envPath)
	s := string(env)
	if strings.Contains(s, "KIS_DEMO_APP_KEY=") || strings.Contains(s, "KIS_REAL_APP_KEY=") || strings.Contains(s, "LS_APP_KEY=") {
		t.Errorf("empty/unchanged fields must not create new lines:\n%s", s)
	}
	if !strings.Contains(s, "KIS_APP_KEY=legacykey\n") || !strings.Contains(s, "KIS_APP_SECRET=legacysecret\n") {
		t.Errorf("legacy lines must be kept:\n%s", s)
	}
	// 모의 시크릿을 바꾸면 구 이름 줄이 갱신된다 (새 줄 추가 아님)
	v.KISDemoSecret = "newsecret"
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}
	env, _ = os.ReadFile(envPath)
	s = string(env)
	if !strings.Contains(s, "KIS_APP_SECRET=newsecret\n") || strings.Contains(s, "KIS_DEMO_APP_SECRET=") {
		t.Errorf("legacy line should be updated in place:\n%s", s)
	}
	// 구 이름 줄과 새 이름 줄이 둘 다 있으면 새 이름 줄을 갱신
	envPath2 := write(t, ".env", "KIS_APP_KEY=old\nKIS_DEMO_APP_KEY=new\n")
	v2, _ := Load(envPath2, yamlPath)
	v2.KISDemoKey = "newer"
	Save(envPath2, yamlPath, v2)
	env, _ = os.ReadFile(envPath2)
	if !strings.Contains(string(env), "KIS_DEMO_APP_KEY=newer\n") || !strings.Contains(string(env), "KIS_APP_KEY=old\n") {
		t.Errorf("new-name line should win:\n%s", env)
	}
}

const legacyEnvYAML = `db_path: data/market.db

kis:
  env: real                    # demo / real
  token_cache: data/token.json

fetch:
  start_date: "2025-09-01"     # 시작일
  daily_at: "04:00"

strategy:
  index_ma_days: 20
`

func TestLoadMigratesLegacyEnvYAMLKey(t *testing.T) {
	v, err := Load(write(t, ".env", sampleEnv), write(t, "config.yaml", legacyEnvYAML))
	if err != nil {
		t.Fatal(err)
	}
	if v.KISEnv != "real" {
		t.Errorf("trade_env should fall back to legacy env: %+v", v)
	}
}

func TestSaveUpdatesLegacyEnvYAMLLineInPlace(t *testing.T) {
	envPath := write(t, ".env", sampleEnv)
	yamlPath := write(t, "config.yaml", legacyEnvYAML)
	v, _ := Load(envPath, yamlPath)
	v.KISEnv = "demo"
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}
	y, _ := os.ReadFile(yamlPath)
	ys := string(y)
	if !strings.Contains(ys, "env: demo                    # demo / real") {
		t.Errorf("legacy env line should be updated in place:\n%s", ys)
	}
	if strings.Contains(ys, "trade_env:") {
		t.Errorf("must not add a new trade_env line:\n%s", ys)
	}
	v2, err := Load(envPath, yamlPath)
	if err != nil || v2.KISEnv != "demo" {
		t.Errorf("reload after legacy save: %+v %v", v2, err)
	}
}

const bothEnvKeysYAML = `db_path: data/market.db

kis:
  trade_env: demo               # demo / real
  env: keepme                   # legacy leftover, should be ignored/untouched
  token_cache: data/token.json

fetch:
  start_date: "2025-09-01"     # 시작일
  daily_at: "04:00"

strategy:
  index_ma_days: 20
`

// TestLoadTradeEnvBeatsLegacyEnvLine: trade_env 와 구 이름 env 줄이 둘 다 있으면 trade_env 가 이긴다.
func TestLoadTradeEnvBeatsLegacyEnvLine(t *testing.T) {
	v, err := Load(write(t, ".env", sampleEnv), write(t, "config.yaml", bothEnvKeysYAML))
	if err != nil {
		t.Fatal(err)
	}
	if v.KISEnv != "demo" {
		t.Errorf("trade_env should win over legacy env line: %+v", v)
	}
}

// TestSaveUpdatesTradeEnvLineNotLegacyWhenBothPresent: 둘 다 있으면 trade_env 줄만 갱신하고
// 구 이름 env 줄은 그대로 둔다.
func TestSaveUpdatesTradeEnvLineNotLegacyWhenBothPresent(t *testing.T) {
	envPath := write(t, ".env", sampleEnv)
	yamlPath := write(t, "config.yaml", bothEnvKeysYAML)
	v, _ := Load(envPath, yamlPath)
	v.KISEnv = "real"
	if err := Save(envPath, yamlPath, v); err != nil {
		t.Fatal(err)
	}
	y, _ := os.ReadFile(yamlPath)
	ys := string(y)
	if !strings.Contains(ys, "trade_env: real") {
		t.Errorf("trade_env line should be updated:\n%s", ys)
	}
	if !strings.Contains(ys, "env: keepme") {
		t.Errorf("legacy env line must be left untouched when trade_env is present:\n%s", ys)
	}
	v2, err := Load(envPath, yamlPath)
	if err != nil || v2.KISEnv != "real" {
		t.Errorf("reload: %+v %v", v2, err)
	}
}
