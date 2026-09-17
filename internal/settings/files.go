package settings

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envKeys 는 .env 에 쓰는 필드와 변수 이름.
var envKeys = map[Field]string{
	KISDemoKey: "KIS_DEMO_APP_KEY", KISDemoSecret: "KIS_DEMO_APP_SECRET", KISDemoAccount: "KIS_DEMO_ACCOUNT",
	KISRealKey: "KIS_REAL_APP_KEY", KISRealSecret: "KIS_REAL_APP_SECRET", KISRealAccount: "KIS_REAL_ACCOUNT",
	LSKey: "LS_APP_KEY", LSSecret: "LS_APP_SECRET",
}

// legacyEnvKeys 는 구 이름. 새 이름 줄이 없고 구 이름 줄이 있으면 그 줄을 갱신한다 (config.Load 의 fallback 과 짝).
var legacyEnvKeys = map[Field]string{KISDemoKey: "KIS_APP_KEY", KISDemoSecret: "KIS_APP_SECRET", KISDemoAccount: "KIS_ACCOUNT"}

// envOrder 는 새로 추가할 때의 순서.
var envOrder = []Field{KISDemoKey, KISDemoSecret, KISDemoAccount, KISRealKey, KISRealSecret, KISRealAccount, LSKey, LSSecret}

// yamlPaths 는 config.yaml 에서 읽을 필드와 경로 (Load 에서 사용).
var yamlPaths = map[Field][]string{
	KISEnv: {"kis", "trade_env"}, DailyAt: {"fetch", "daily_at"}, StartDate: {"fetch", "start_date"},
}

// yamlLegacyPaths 는 새 경로 줄이 없을 때 대신 읽는 구 경로 (config.Load 의 trade_env/env 마이그레이션과 짝).
// Save 는 새 이름 줄이 없고 구 이름 줄이 있으면 그 줄을 갱신한다 (setYAMLLine 의 fallback key).
var yamlLegacyPaths = map[Field][]string{
	KISEnv: {"kis", "env"},
}

// yamlQuoted 는 Save 에서 config.yaml 값을 따옴표로 감쌀지 여부.
var yamlQuoted = map[Field]bool{
	DailyAt: true, StartDate: true,
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
	// 구 이름(KIS_APP_KEY 등)만 있는 .env 는 모의 칸으로 보여준다 (config.Load 의 fallback 과 같은 뜻).
	for f, legacy := range map[Field]string{KISDemoKey: "KIS_APP_KEY", KISDemoSecret: "KIS_APP_SECRET", KISDemoAccount: "KIS_ACCOUNT"} {
		if v.Get(f) == "" {
			if val, ok := envValue(lines, legacy); ok {
				v.Set(f, val)
			}
		}
	}
	root, err := readYAML(yamlPath)
	if err != nil {
		return v, err
	}
	for f, path := range yamlPaths {
		if n := findNode(root, path); n != nil {
			v.Set(f, n.Value)
		} else if legacy, ok := yamlLegacyPaths[f]; ok {
			if n := findNode(root, legacy); n != nil {
				v.Set(f, n.Value)
			}
		}
	}
	return v, nil
}

// Save 는 모든 필드를 검증한 뒤 .env 와 config.yaml 을 고쳐 쓴다. 검증 실패면 아무것도 쓰지 않는다.
//
// 두 파일의 내용을 모두 메모리에서 준비한 뒤 config.yaml, .env 순서로 원자적으로 쓴다.
// config.yaml 쓰기가 실패하면 아무 파일도 바뀌지 않는다. .env 쓰기(rename)가
// config.yaml 성공 이후 실패하면 config.yaml 은 이미 갱신된 채로 남고 에러를 반환한다 —
// 두 파일을 하나의 트랜잭션으로 묶을 방법이 없어 감수하는 순서다.
func Save(envPath, yamlPath string, v Values) error {
	for f := Field(0); f < FieldCount; f++ {
		if err := Validate(f, v.Get(f)); err != nil {
			return fmt.Errorf("%s: %w", Labels[f], err)
		}
	}
	envLines, err := readLines(envPath)
	if err != nil {
		return err
	}
	for _, f := range envOrder {
		envLines = setEnv(envLines, envKeys[f], legacyEnvKeys[f], v.Get(f))
	}

	yamlLines, err := readLines(yamlPath)
	if err != nil {
		return err
	}
	for f, path := range yamlPaths {
		fallbackKey := ""
		if legacy, ok := yamlLegacyPaths[f]; ok {
			fallbackKey = legacy[1]
		}
		yamlLines, err = setYAMLLine(yamlLines, path[0], path[1], fallbackKey, v.Get(f), yamlQuoted[f])
		if err != nil {
			return fmt.Errorf("settings: config.yaml: %w", err)
		}
	}

	yamlData := []byte(strings.Join(yamlLines, "\n") + "\n")
	envData := []byte(strings.Join(envLines, "\n") + "\n")

	if err := writeAtomic(yamlPath, yamlData, 0o644); err != nil {
		return fmt.Errorf("settings: config.yaml: %w", err)
	}
	if err := writeAtomic(envPath, envData, 0o600); err != nil {
		return fmt.Errorf("settings: .env: %w", err)
	}
	return nil
}

// writeAtomic 은 path+".tmp" 에 쓰고 fsync 한 뒤 path 로 rename 한다.
// 중간에 실패하면 tmp 파일을 지운다.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
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
// setEnv 는 key 줄이 있으면 그 자리에 KEY=value 로 바꾼다. 없고 legacy 줄이 있으면 그 줄을 갱신한다.
// 둘 다 없으면 값이 있을 때만 끝에 붙인다 — 빈 값을 새 줄로 만들면 구 이름 값을 가리기 때문.
func setEnv(lines []string, key, legacy, value string) []string {
	for i, line := range lines {
		if k, _, ok := splitEnv(line); ok && k == key {
			lines[i] = key + "=" + value
			return lines
		}
	}
	if legacy != "" {
		for i, line := range lines {
			if k, _, ok := splitEnv(line); ok && k == legacy {
				lines[i] = legacy + "=" + value
				return lines
			}
		}
	}
	if value == "" {
		return lines
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

var topLevelRe = regexp.MustCompile(`^\S`)

// setYAMLLine 은 config.yaml 을 줄 단위로 편집해 section 아래 key 의 값을 바꾼다.
// key 줄이 없고 fallbackKey 가 있으면 그 줄(구 이름)을 대신 찾아 값만 갱신한다 (이름은 그대로 둔다).
// 들여쓰기와 줄 끝 주석의 정렬(가능하면 원래 컬럼 유지, 값이 길어지면 공백 2칸)을 그대로 둔다.
// section 이나 key(및 fallbackKey)를 찾지 못하면 에러(우리 config.yaml 은 항상 이 필드를 갖고 있다고 가정).
func setYAMLLine(lines []string, section, key, fallbackKey, value string, quoted bool) ([]string, error) {
	sectionRe := regexp.MustCompile(`^` + regexp.QuoteMeta(section) + `:\s*(#.*)?$`)

	secIdx := -1
	for i, l := range lines {
		if sectionRe.MatchString(l) {
			secIdx = i
			break
		}
	}
	if secIdx == -1 {
		return nil, fmt.Errorf("section %q not found", section)
	}

	end := len(lines)
	for i := secIdx + 1; i < len(lines); i++ {
		if lines[i] != "" && topLevelRe.MatchString(lines[i]) {
			end = i
			break
		}
	}

	trySet := func(useKey string) bool {
		keyRe := regexp.MustCompile(`^(\s+)` + regexp.QuoteMeta(useKey) + `:\s*([^#]*?)\s*(#.*)?$`)
		for i := secIdx + 1; i < end; i++ {
			m := keyRe.FindStringSubmatch(lines[i])
			if m == nil {
				continue
			}
			indent, comment := m[1], m[3]
			newVal := value
			if quoted {
				newVal = `"` + value + `"`
			}
			newLine := indent + useKey + ": " + newVal
			if comment != "" {
				hashCol := strings.Index(lines[i], "#")
				pad := hashCol - len(newLine)
				if pad < 2 {
					pad = 2
				}
				newLine += strings.Repeat(" ", pad) + comment
			}
			lines[i] = newLine
			return true
		}
		return false
	}

	if trySet(key) {
		return lines, nil
	}
	if fallbackKey != "" && trySet(fallbackKey) {
		return lines, nil
	}
	return nil, fmt.Errorf("key %q not found in section %q", key, section)
}
