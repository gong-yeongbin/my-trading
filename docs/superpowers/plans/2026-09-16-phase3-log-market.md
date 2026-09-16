# 3단계: 로그 파일·로그 패널·장 상태 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 프로그램이 하는 일이 `data/trader.log`에 JSON 한 줄씩 남고, TUI 로그 패널이 그 파일을 1초마다 따라간다. 하단 지수 줄 앞에 장 상태(`[장중]` 등)가 시계 기준으로 보이고, LS 장운영정보(`JIF`)가 오면 그 값으로 덮어쓰며 로그에도 남는다.

**Architecture:** `logfile`(slog JSON 파일 로거 열기, 줄 파싱, 꼬리 읽기, 증분 읽기) ← `tui/run.go`가 로거를 만들어 `ls`에 `kind=연결`로 넘기고, 파일을 1초마다 읽어 `LogMsg`로 밀어 넣음. `market`(시계 기반 장 상태, JIF 코드표 — 순수 함수) ← `ls`가 `JIF`를 `MarketStatus` 이벤트로 파싱, `run.go`가 코드표로 바꿔 `MarketStatusMsg` + 로그. TUI 는 표시만.

**Tech Stack:** Go 1.27 표준 `log/slog`, `encoding/json`, `os`. 새 외부 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (10.1절 하단 줄, 10.2절 JIF, 10.3절 로그 파일, 10.4절, 11절 로그 항목)

**선행:** 2단계 완료 (`internal/ls` 에 `Raw` 이벤트, `tui` 에 `ConnectedMsg`·`DisconnectedMsg`, `config.Log.File`).

## Global Constraints

- 모듈 경로: `github.com/gong-yeongbin/my-trading`. 새 외부 의존성 없음. 테스트는 표준 `testing`만.
- 로그 파일(스펙 10.3): `log/slog` JSON 핸들러, 한 줄에 한 레코드, 필드 `time`, `level`, `msg`, `kind`(수집 / 지수 / 매매 / 연결 / 오류) + 임의 속성. append 모드, 파일 0600, 디렉터리 0700. 로테이션 없음.
- TUI 로그 패널: 시작 시 마지막 200줄, 이후 1초마다 파일 크기를 확인해 늘어난 만큼 읽음. 최신순. 버퍼 200줄 유지.
- 장 상태 문구(시계 기준, KST, 공휴일 미반영): 토·일 `휴장`; 평일 09:00 전 `장전`; 09:00~15:20 `장중`; 15:20~15:30 `동시호가`; 15:30~16:00 `장마감`; 16:00~18:00 `시간외`; 18:00 이후 `장마감`.
- JIF 코드표(xingAPI 관례, 실서버 관측 후 보정): 11 → `장전`(장전 동시호가), 21 → `장중`, 31 → `동시호가`, 41 → `장마감`, 51 → `시간외`(시간외 종가), 52 → `장마감`, 61 → `시간외`(시간외 단일가), 62 → `장마감`. 그 외 코드는 표시에 쓰지 않고 로그만. `jangubun` 1 코스피, 2 코스닥.
- TUI 는 로직을 갖지 않는다. 장 상태 판정은 `market` 패키지, JIF→문구 변환은 `run.go`에서.
- 로그 파일을 열 수 없어도 TUI 는 종료하지 않는다: 로거는 폐기, 로그 패널에 `[오류] 로그 파일 열기 실패: …` 한 줄.
- 한글 폭은 `lipgloss.Width`/`ansi.Truncate`만. 색·굵기 스타일 금지.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/*.json`은 읽기 금지. `data/trader.log`는 읽어도 된다 (비밀 없음).
- 파일 편집 후 `gofmt -l .` 결과가 비어 있어야 한다. `go run ./cmd/trader`(인자 없음)는 실행하지 않는다.

---

## 파일 구조

```
internal/logfile/logfile.go     Open, Line, ParseLine, Tail, Reader
internal/logfile/logfile_test.go
internal/market/market.go       Status(now), FromJIF(code)
internal/market/market_test.go
internal/ls/client.go           MarketStatus 이벤트 추가 (수정)
internal/ls/parse.go            JIF 파싱 (수정)
internal/ls/parse_test.go       JIF 테스트 (수정)
internal/tui/types.go           MarketStatusMsg 추가, LogLine 유지 (수정)
internal/tui/model.go           marketStatus 상태·판정 (수정)
internal/tui/view.go            하단 줄에 [상태] (수정)
internal/tui/logwatch.go        파일 꼬리 → LogMsg
internal/tui/logwatch_test.go
internal/tui/run.go             로거 생성, ls 에 kind=연결, JIF 처리, 로그 감시 (수정)
internal/tui/ls.go              MarketStatus → nil (run.go 가 직접 처리) (수정)
internal/tui/fake.go            LogMsg 항목 삭제 (수정)
internal/tui/model_test.go, view_test.go, ls_test.go   테스트 추가 (수정)
```

---

### Task 1: 로그 파일 패키지 (internal/logfile)

**Files:**
- Create: `internal/logfile/logfile.go`
- Test: `internal/logfile/logfile_test.go`

**Interfaces:**
- Produces: `logfile.Open(path string) (*slog.Logger, io.Closer, error)`, `logfile.Line{Time time.Time; Level, Kind, Msg string}`, `logfile.ParseLine([]byte) (Line, bool)`, `logfile.Tail(path string, n int) (lines []Line, size int64, err error)` (파일 순서, 없으면 빈 결과·0·nil), `logfile.NewReader(path string, offset int64) *Reader`, `(*Reader).Next() ([]Line, error)` (offset 이후의 완성된 줄들, 파일 순서). Task 3 의 `logwatch.go`와 `run.go`가 쓴다. 6단계 `universe`/`fetch`도 `Open`을 쓴다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/logfile/logfile_test.go`:

```go
package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "trader.log")
	logger, closer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.With("kind", "연결").Info("ls websocket 연결", "subs", 5)
	logger.Warn("실패", "kind", "오류", "err", "boom")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
	b, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d: %s", len(lines), b)
	}
	first, ok := ParseLine([]byte(lines[0]))
	if !ok || first.Kind != "연결" || first.Level != "INFO" || first.Msg != "ls websocket 연결 subs=5" {
		t.Errorf("first = %+v ok=%v", first, ok)
	}
	if time.Since(first.Time) > time.Minute || time.Since(first.Time) < 0 {
		t.Errorf("time not recent: %v", first.Time)
	}
	second, ok := ParseLine([]byte(lines[1]))
	if !ok || second.Kind != "오류" || second.Level != "WARN" || second.Msg != "실패 err=boom" {
		t.Errorf("second = %+v ok=%v", second, ok)
	}

	// append: 다시 열면 이어 쓴다
	logger2, closer2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	logger2.Info("세 번째")
	closer2.Close()
	b, _ = os.ReadFile(path)
	if strings.Count(string(b), "\n") != 3 {
		t.Errorf("append failed: %s", b)
	}
}

func TestParseLineRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "not json", `{"level":"INFO"}`, `{"time":"bad","msg":"x"}`} {
		if l, ok := ParseLine([]byte(s)); ok {
			t.Errorf("expected reject for %q, got %+v", s, l)
		}
	}
	l, ok := ParseLine([]byte(`{"time":"2026-09-16T09:00:01.5+09:00","level":"INFO","msg":"시작"}`))
	if !ok || l.Kind != "" || l.Msg != "시작" || l.Time.Hour() != 9 {
		t.Errorf("minimal line = %+v ok=%v", l, ok)
	}
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func line(msg string) string {
	return `{"time":"2026-09-16T10:00:00+09:00","level":"INFO","msg":"` + msg + `","kind":"수집"}`
}

func TestTailLastN(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.log")
	lines, size, err := Tail(path, 3)
	if err != nil || len(lines) != 0 || size != 0 {
		t.Fatalf("missing file: %v %v %d", lines, err, size)
	}
	writeLines(t, path, line("a"), line("b"), "garbage", line("c"), line("d"))
	lines, size, err = Tail(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, l := range lines {
		got = append(got, l.Msg)
	}
	if strings.Join(got, ",") != "b,c,d" {
		t.Errorf("tail = %v", got)
	}
	if info, _ := os.Stat(path); size != info.Size() {
		t.Errorf("size = %d, want %d", size, info.Size())
	}
}

func TestReaderIncrementalAndPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.log")
	writeLines(t, path, line("old"))
	_, size, _ := Tail(path, 10)
	r := NewReader(path, size)

	got, err := r.Next()
	if err != nil || len(got) != 0 {
		t.Fatalf("nothing new expected: %v %v", got, err)
	}
	writeLines(t, path, line("one"), line("two"))
	got, err = r.Next()
	if err != nil || len(got) != 2 || got[0].Msg != "one" || got[1].Msg != "two" {
		t.Fatalf("incremental = %+v %v", got, err)
	}
	// 부분 줄: 개행 없이 끝난 조각은 다음 Next 까지 보류
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	f.WriteString(`{"time":"2026-09-16T10:00:00+09:00","level":"INFO",`)
	f.Close()
	got, err = r.Next()
	if err != nil || len(got) != 0 {
		t.Fatalf("partial line should be held: %+v %v", got, err)
	}
	f, _ = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	f.WriteString(`"msg":"three"}` + "\n")
	f.Close()
	got, err = r.Next()
	if err != nil || len(got) != 1 || got[0].Msg != "three" {
		t.Fatalf("completed partial = %+v %v", got, err)
	}
	// 파일이 잘리면(truncate) 처음부터 다시 읽는다
	os.WriteFile(path, []byte(line("fresh")+"\n"), 0o600)
	got, err = r.Next()
	if err != nil || len(got) != 1 || got[0].Msg != "fresh" {
		t.Fatalf("after truncate = %+v %v", got, err)
	}
	// 파일이 사라지면 오류 없이 빈 결과
	os.Remove(path)
	if got, err := r.Next(); err != nil || len(got) != 0 {
		t.Fatalf("missing file = %+v %v", got, err)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/logfile/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: Open`)

- [ ] **Step 3: 구현**

`internal/logfile/logfile.go`:

```go
// Package logfile 은 서브커맨드와 TUI 가 함께 쓰는 JSON 줄 로그 파일을 열고 읽는다.
// 한 줄 = slog 레코드 하나: time, level, msg, kind(수집/지수/매매/연결/오류) + 속성.
package logfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Open 은 path 를 append 모드로 열어 JSON 핸들러 로거를 돌려준다. 디렉터리가 없으면 만든다.
func Open(path string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, fmt.Errorf("logfile: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("logfile: %w", err)
	}
	return slog.New(slog.NewJSONHandler(f, nil)), f, nil
}

// Line 은 로그 한 줄. Msg 에는 msg 뒤에 나머지 속성이 " key=value" 로 붙는다.
type Line struct {
	Time  time.Time
	Level string
	Kind  string
	Msg   string
}

// ParseLine 은 JSON 한 줄을 Line 으로 바꾼다. time 이나 msg 가 없거나 깨졌으면 false.
func ParseLine(b []byte) (Line, bool) {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return Line{}, false
	}
	ts, _ := raw["time"].(string)
	msg, _ := raw["msg"].(string)
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil || msg == "" {
		return Line{}, false
	}
	level, _ := raw["level"].(string)
	kind, _ := raw["kind"].(string)
	keys := make([]string, 0, len(raw))
	for k := range raw {
		switch k {
		case "time", "level", "msg", "kind":
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(msg)
	for _, k := range keys {
		fmt.Fprintf(&sb, " %s=%v", k, raw[k])
	}
	return Line{Time: t, Level: level, Kind: kind, Msg: sb.String()}, true
}

// Tail 은 파일의 마지막 n 줄(파싱되는 것만)을 파일 순서로 돌려주고, 파일 크기도 돌려준다. 파일이 없으면 빈 결과.
func Tail(path string, n int) ([]Line, int64, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	lines := parseLines(b)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, int64(len(b)), nil
}

// Reader 는 offset 이후로 늘어난 부분만 읽는다. 개행으로 끝나지 않은 조각은 다음 호출까지 보류한다.
type Reader struct {
	path    string
	offset  int64
	partial []byte
}

func NewReader(path string, offset int64) *Reader {
	return &Reader{path: path, offset: offset}
}

// Next 는 새로 완성된 줄들을 파일 순서로 돌려준다. 파일이 없으면 빈 결과, 잘렸으면 처음부터 다시 읽는다.
func (r *Reader) Next() ([]Line, error) {
	f, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < r.offset {
		r.offset, r.partial = 0, nil
	}
	if info.Size() == r.offset {
		return nil, nil
	}
	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return nil, err
	}
	chunk, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	r.offset += int64(len(chunk))
	buf := append(r.partial, chunk...)
	last := bytes.LastIndexByte(buf, '\n')
	if last < 0 {
		r.partial = buf
		return nil, nil
	}
	r.partial = append([]byte(nil), buf[last+1:]...)
	return parseLines(buf[:last+1]), nil
}

func parseLines(b []byte) []Line {
	var out []Line
	for _, raw := range bytes.Split(b, []byte{'\n'}) {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			continue
		}
		if l, ok := ParseLine(raw); ok {
			out = append(out, l)
		}
	}
	return out
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./internal/logfile/ && go test ./internal/logfile/ -v 2>&1 | tail -12`
Expected: 4개 테스트 PASS

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/logfile
git commit -m "feat(logfile): JSON 줄 로그 열기·파싱·꼬리 읽기"
```

---

### Task 2: 장 상태 (internal/market) + JIF 파싱 (internal/ls)

**Files:**
- Create: `internal/market/market.go`, `internal/market/market_test.go`
- Modify: `internal/ls/client.go`, `internal/ls/parse.go`, `internal/ls/parse_test.go`

**Interfaces:**
- Produces: `market.Status(t time.Time) string` (KST 기준 문구), `market.FromJIF(code string) (string, bool)`, `market.KST *time.Location`; `ls.MarketStatus{Market, Code string}` 이벤트 (`Market` 은 `kospi`/`kosdaq`/원문). Task 3 의 `run.go`가 셋 다 쓴다.

- [ ] **Step 1: market 테스트 작성**

`internal/market/market_test.go`:

```go
package market

import (
	"testing"
	"time"
)

func at(day int, hh, mm int) time.Time {
	// 2026-09-14 월요일. day 는 그 주의 날짜.
	return time.Date(2026, 9, day, hh, mm, 0, 0, KST)
}

func TestStatusByClock(t *testing.T) {
	cases := []struct {
		t    time.Time
		want string
	}{
		{at(14, 8, 59), "장전"},
		{at(14, 9, 0), "장중"},
		{at(14, 15, 19), "장중"},
		{at(14, 15, 20), "동시호가"},
		{at(14, 15, 29), "동시호가"},
		{at(14, 15, 30), "장마감"},
		{at(14, 15, 59), "장마감"},
		{at(14, 16, 0), "시간외"},
		{at(14, 17, 59), "시간외"},
		{at(14, 18, 0), "장마감"},
		{at(14, 23, 30), "장마감"},
		{at(19, 10, 0), "휴장"}, // 토
		{at(20, 10, 0), "휴장"}, // 일
	}
	for _, tc := range cases {
		if got := Status(tc.t); got != tc.want {
			t.Errorf("Status(%v) = %q, want %q", tc.t.Format("Mon 15:04"), got, tc.want)
		}
	}
	// 다른 시간대로 들어와도 KST 로 판정한다
	utc := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) // KST 10:00
	if got := Status(utc); got != "장중" {
		t.Errorf("UTC input: %q", got)
	}
}

func TestFromJIF(t *testing.T) {
	cases := map[string]string{"11": "장전", "21": "장중", "31": "동시호가", "41": "장마감", "51": "시간외", "52": "장마감", "61": "시간외", "62": "장마감"}
	for code, want := range cases {
		got, ok := FromJIF(code)
		if !ok || got != want {
			t.Errorf("FromJIF(%s) = %q,%v want %q", code, got, ok, want)
		}
	}
	for _, code := range []string{"", "22", "99", "abc"} {
		if got, ok := FromJIF(code); ok {
			t.Errorf("FromJIF(%q) should be unknown, got %q", code, got)
		}
	}
}
```

- [ ] **Step 2: market 구현**

`internal/market/market.go`:

```go
// Package market 은 국내 주식 장 상태를 판정한다. 시계 기준 판정과 LS 장운영정보(JIF) 코드표를 제공한다.
// 공휴일은 반영하지 않는다 (JIF 가 오면 그 값이 우선).
package market

import "time"

// KST 는 tzdata 없이도 동작하도록 고정 오프셋으로 둔다.
var KST = time.FixedZone("KST", 9*60*60)

// Status 는 t 시각의 장 상태 문구: 휴장 / 장전 / 장중 / 동시호가 / 장마감 / 시간외.
func Status(t time.Time) string {
	t = t.In(KST)
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return "휴장"
	}
	hm := t.Hour()*60 + t.Minute()
	switch {
	case hm < 9*60:
		return "장전"
	case hm < 15*60+20:
		return "장중"
	case hm < 15*60+30:
		return "동시호가"
	case hm < 16*60:
		return "장마감"
	case hm < 18*60:
		return "시간외"
	default:
		return "장마감"
	}
}

// jifStatus 는 LS 장운영정보 jstatus 코드 → 문구. xingAPI 관례이며 실서버 관측 후 보정한다.
var jifStatus = map[string]string{
	"11": "장전",     // 장전 동시호가 시작
	"21": "장중",     // 장 시작
	"31": "동시호가", // 장 마감 동시호가 시작
	"41": "장마감",   // 장 마감
	"51": "시간외",   // 시간외 종가 매매 시작
	"52": "장마감",   // 시간외 종가 매매 종료
	"61": "시간외",   // 시간외 단일가 매매 시작
	"62": "장마감",   // 시간외 단일가 매매 종료
}

// FromJIF 는 코드가 표에 있으면 문구와 true.
func FromJIF(code string) (string, bool) {
	s, ok := jifStatus[code]
	return s, ok
}
```

Run: `go test ./internal/market/ -v 2>&1 | tail -6`
Expected: 2개 PASS

- [ ] **Step 3: ls JIF 테스트 추가**

`internal/ls/parse_test.go` 에서 `TestParseUnknownTRAsRaw` 를 다른 TR 로 바꾸고 JIF 테스트를 추가한다:

```go
func TestParseUnknownTRAsRaw(t *testing.T) {
	ev, ok := parseMessage([]byte(`{"header":{"tr_cd":"S3_","tr_key":"005930"},"body":{"price":"71200"}}`))
	if !ok {
		t.Fatal("expected Raw event")
	}
	r, isRaw := ev.(Raw)
	if !isRaw || r.TrCd != "S3_" || r.TrKey != "005930" || r.Body["price"] != "71200" {
		t.Errorf("raw = %+v", ev)
	}
}

func TestParseJIF(t *testing.T) {
	cases := []struct {
		key        string
		body       string
		wantMarket string
		wantCode   string
	}{
		{"1", `{"jangubun":"1","jstatus":"21"}`, "kospi", "21"},
		{"2", `{"jangubun":"2","jstatus":"41"}`, "kosdaq", "41"},
		{"5", `{"jangubun":"5","jstatus":"21"}`, "5", "21"}, // 모르는 시장은 원문
	}
	for _, tc := range cases {
		ev, ok := parseMessage([]byte(`{"header":{"tr_cd":"JIF","tr_key":"` + tc.key + `"},"body":` + tc.body + `}`))
		if !ok {
			t.Fatalf("expected MarketStatus for %s", tc.body)
		}
		ms, isMS := ev.(MarketStatus)
		if !isMS || ms.Market != tc.wantMarket || ms.Code != tc.wantCode {
			t.Errorf("parse %s = %+v", tc.body, ev)
		}
	}
	if _, ok := parseMessage([]byte(`{"header":{"tr_cd":"JIF","tr_key":"1"},"body":{"jangubun":"1"}}`)); ok {
		t.Error("JIF without jstatus should be ignored")
	}
}
```

- [ ] **Step 4: ls 구현**

`internal/ls/client.go` 의 `Raw` 타입 앞에 추가:

```go
// MarketStatus 는 JIF 장운영정보 한 건. Market 은 kospi / kosdaq / (모르면 jangubun 원문). Code 는 jstatus 코드.
type MarketStatus struct {
	Market, Code string
}

func (MarketStatus) isEvent() {}
```

`internal/ls/parse.go` 의 `switch m.Header.TrCd` 에 `default:` 앞에 케이스 추가:

```go
	case "JIF":
		code := field(m.Body, "jstatus")
		if code == "" {
			return nil, false
		}
		market := field(m.Body, "jangubun")
		switch market {
		case "1":
			market = "kospi"
		case "2":
			market = "kosdaq"
		}
		return MarketStatus{Market: market, Code: code}, true
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./internal/ls/ ./internal/market/ && go test ./internal/ls/ ./internal/market/ 2>&1 | tail -3`
Expected: 둘 다 ok

- [ ] **Step 6: 커밋 요청**

```bash
git add internal/market internal/ls
git commit -m "feat: 장 상태 판정(market), LS JIF 장운영정보 파싱"
```

---

### Task 3: TUI — 장 상태 표시, 로그 패널 실데이터, 로거 배선

**Files:**
- Modify: `internal/tui/types.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/ls.go`, `internal/tui/run.go`, `internal/tui/fake.go`, `internal/tui/model_test.go`, `internal/tui/view_test.go`, `internal/tui/ls_test.go`
- Create: `internal/tui/logwatch.go`, `internal/tui/logwatch_test.go`

**Interfaces:**
- Consumes: `logfile.Open/Tail/NewReader/Line`, `market.Status/FromJIF`, `ls.MarketStatus`, `config.Log.File`.
- Produces: `tui.MarketStatusMsg{Status string}`, 비공개 `watchLog(ctx, r *logfile.Reader, initial []logfile.Line, every time.Duration, send func(tea.Msg))`, `toLogLines([]logfile.Line) []LogLine`(최신순). 8단계는 `run.go`의 `logger` 를 잔고 폴링에도 넘긴다.

- [ ] **Step 1: 실패하는 테스트 추가**

`internal/tui/model_test.go` 끝에 추가:

```go
func TestMarketStatusFromClockAndJIF(t *testing.T) {
	m := sized(t)
	tue1000 := time.Date(2026, 9, 15, 10, 0, 0, 0, market.KST)
	m = send(m, tickMsg(tue1000))
	if !strings.Contains(m.View(), "[장중]") {
		t.Errorf("clock-based status missing:\n%s", m.View())
	}
	m = send(m, MarketStatusMsg{Status: "동시호가"})
	if !strings.Contains(m.View(), "[동시호가]") {
		t.Errorf("JIF status should override:\n%s", m.View())
	}
	// 같은 날 시계가 흘러도 JIF 값 유지
	m = send(m, tickMsg(tue1000.Add(time.Hour)))
	if !strings.Contains(m.View(), "[동시호가]") {
		t.Errorf("JIF status should persist within the day:\n%s", m.View())
	}
	// 날짜가 바뀌면 시계 기준으로 복귀
	m = send(m, tickMsg(tue1000.Add(24*time.Hour)))
	if !strings.Contains(m.View(), "[장중]") {
		t.Errorf("next day should fall back to clock:\n%s", m.View())
	}
}
```
(`model_test.go` import 에 `"github.com/gong-yeongbin/my-trading/internal/market"` 추가.)

`internal/tui/view_test.go` 끝에 추가:

```go
func TestViewFooterStatusPrefix(t *testing.T) {
	m := sized(t)
	m = send(m, tickMsg(time.Date(2026, 9, 19, 10, 0, 0, 0, market.KST))) // 토
	m = send(m, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008})
	if !strings.Contains(m.View(), " [휴장] 코스피 2,712.40 ▲+0.8%") {
		t.Errorf("footer should start with status:\n%s", m.View())
	}
}
```
(`view_test.go` import 에 `market` 추가.)

`internal/tui/ls_test.go` 의 `TestLSToMsg` 표에 추가:

```go
		{"market-status", ls.MarketStatus{Market: "kospi", Code: "21"}, nil}, // run.go 가 직접 처리
```

`internal/tui/logwatch_test.go` (새 파일):

```go
package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/logfile"
)

func TestToLogLinesNewestFirstCapped(t *testing.T) {
	var in []logfile.Line
	for i := 0; i < 205; i++ {
		in = append(in, logfile.Line{Time: time.Unix(int64(i), 0), Kind: "수집", Msg: "m"})
	}
	out := toLogLines(in)
	if len(out) != logKeep {
		t.Fatalf("len = %d, want %d", len(out), logKeep)
	}
	if out[0].Time.Unix() != 204 || out[len(out)-1].Time.Unix() != 5 {
		t.Errorf("order/cap wrong: first=%d last=%d", out[0].Time.Unix(), out[len(out)-1].Time.Unix())
	}
}

func TestWatchLogSendsInitialThenIncrements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.log")
	write := func(msg string) {
		f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		f.WriteString(`{"time":"2026-09-16T10:00:00+09:00","level":"INFO","msg":"` + msg + `","kind":"연결"}` + "\n")
		f.Close()
	}
	write("첫줄")
	initial, size, _ := logfile.Tail(path, logKeep)
	msgs := make(chan tea.Msg, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		watchLog(ctx, logfile.NewReader(path, size), initial, 10*time.Millisecond, func(m tea.Msg) { msgs <- m })
		close(done)
	}()

	first := (<-msgs).(LogMsg)
	if len(first.Lines) != 1 || first.Lines[0].Msg != "첫줄" || first.Lines[0].Kind != "연결" {
		t.Fatalf("initial = %+v", first)
	}
	write("둘째")
	select {
	case m := <-msgs:
		lm := m.(LogMsg)
		if len(lm.Lines) != 2 || lm.Lines[0].Msg != "둘째" || lm.Lines[1].Msg != "첫줄" {
			t.Fatalf("incremental = %+v", lm)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no LogMsg after append")
	}
	// 변화 없으면 보내지 않는다
	select {
	case m := <-msgs:
		t.Fatalf("unexpected message without change: %+v", m)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchLog did not stop on cancel")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/tui/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: MarketStatusMsg`, `toLogLines`, `watchLog`, `logKeep`)

- [ ] **Step 3: types.go**

`internal/tui/types.go` 의 `ConnectedMsg` 앞에 추가:

```go
// MarketStatusMsg 는 LS 장운영정보를 문구로 바꾼 것 (장전/장중/동시호가/장마감/시간외). 그날 안에서는 시계 판정보다 우선한다.
type MarketStatusMsg struct {
	Status string
}
```

- [ ] **Step 4: model.go**

`Model` 구조체에 필드 추가 (`linkOK` 아래):

```go
	jifStatus string    // JIF 로 받은 장 상태. 비어 있으면 시계 기준
	jifAt     time.Time // jifStatus 를 받은 시각
```

`Update` 에 케이스 추가 (`case ConnectedMsg:` 앞):

```go
	case MarketStatusMsg:
		m.jifStatus, m.jifAt = msg.Status, m.now
```

`model.go` 끝에 추가:

```go
// marketStatus 는 하단 줄에 붙일 장 상태. 같은 날 JIF 를 받았으면 그 값, 아니면 시계 기준.
func (m Model) marketStatus() string {
	if m.jifStatus != "" {
		a, b := m.jifAt.In(market.KST), m.now.In(market.KST)
		if a.Year() == b.Year() && a.YearDay() == b.YearDay() {
			return m.jifStatus
		}
	}
	return market.Status(m.now)
}
```
(`model.go` import 에 `"github.com/gong-yeongbin/my-trading/internal/market"` 추가.)

- [ ] **Step 5: view.go 하단 줄**

`footerLeft` 를 이렇게 바꾼다:

```go
func (m Model) footerLeft() string {
	return " [" + m.marketStatus() + "] 코스피 " + fmtIndex(m.kospi, m.linkOK) + "   코스닥 " + fmtIndex(m.kosdaq, m.linkOK)
}
```

- [ ] **Step 6: logwatch.go**

`internal/tui/logwatch.go`:

```go
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/logfile"
)

// logKeep 은 로그 패널이 들고 있는 최대 줄 수.
const logKeep = 200

// toLogLines 는 파일 순서의 줄들을 최신순 LogLine 으로 바꾸고 logKeep 개로 자른다.
func toLogLines(lines []logfile.Line) []LogLine {
	if len(lines) > logKeep {
		lines = lines[len(lines)-logKeep:]
	}
	out := make([]LogLine, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		out = append(out, LogLine{Time: lines[i].Time.Local(), Kind: lines[i].Kind, Msg: lines[i].Msg})
	}
	return out
}

// watchLog 는 initial 로 LogMsg 를 한 번 보낸 뒤, every 마다 r.Next() 로 새 줄을 읽어 늘어났을 때만 LogMsg 를 보낸다. ctx 가 끝나면 반환.
func watchLog(ctx context.Context, r *logfile.Reader, initial []logfile.Line, every time.Duration, send func(tea.Msg)) {
	buf := append([]logfile.Line(nil), initial...)
	send(LogMsg{Lines: toLogLines(buf)})
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			more, err := r.Next()
			if err != nil || len(more) == 0 {
				continue
			}
			buf = append(buf, more...)
			if len(buf) > logKeep {
				buf = buf[len(buf)-logKeep:]
			}
			send(LogMsg{Lines: toLogLines(buf)})
		}
	}
}
```

- [ ] **Step 7: ls.go — MarketStatus 는 run.go 가 직접 처리**

`lsToMsg` 는 손대지 않는다 (`MarketStatus` 는 `default` 로 nil). 테이블 테스트 케이스만 Step 1 에서 추가했다.

- [ ] **Step 8: run.go 배선**

`internal/tui/run.go` 전체를 이렇게 바꾼다:

```go
package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/logfile"
	"github.com/gong-yeongbin/my-trading/internal/ls"
	"github.com/gong-yeongbin/my-trading/internal/market"
)

// Run 은 전체 화면 TUI 를 띄우고 종료될 때까지 막는다.
// 로그는 cfg.Log.File 에 쓰고 로그 패널이 그 파일을 따라간다. LS 앱키가 있으면 실시간 뉴스·지수·장운영정보를 구독한다.
func Run(ctx context.Context, cfg *config.Config) error {
	// 취소 가능한 자식 컨텍스트: Run 안에서 시작하는 고루틴들이 q 로 TUI 종료 시 함께 멈추도록.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithContext(ctx))

	// 로그 파일. 못 열면 TUI 는 계속 뜨고 로그 패널에 오류 한 줄만 보인다 (스펙 11).
	logger, closer, err := logfile.Open(cfg.Log.File)
	var initial []logfile.Line
	var size int64
	if err != nil {
		logger = slog.New(slog.DiscardHandler)
		initial = []logfile.Line{{Time: time.Now(), Level: "ERROR", Kind: "오류", Msg: "로그 파일 열기 실패: " + err.Error()}}
	} else {
		defer closer.Close()
		initial, size, _ = logfile.Tail(cfg.Log.File, logKeep)
	}
	go watchLog(ctx, logfile.NewReader(cfg.Log.File, size), initial, time.Second, p.Send)

	if cfg.LS.HasAppKey() {
		client := ls.New(ls.Config{
			BaseURL: cfg.LS.BaseURL, WSURL: cfg.LS.WSURL,
			AppKey: cfg.LS.AppKey, AppSecret: cfg.LS.AppSecret, TokenCache: cfg.LS.TokenCache,
		}, logger.With("kind", "연결"))
		events := make(chan ls.Event, 64)
		go client.Run(ctx, lsSubscriptions, events)
		indexLog := logger.With("kind", "지수")
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ev := <-events:
					if ms, ok := ev.(ls.MarketStatus); ok {
						status, known := market.FromJIF(ms.Code)
						indexLog.Info(fmt.Sprintf("%s 장운영 %s", ms.Market, ms.Code), "status", status, "known", known)
						if known {
							p.Send(MarketStatusMsg{Status: status})
						}
						continue
					}
					if msg := lsToMsg(ev); msg != nil {
						p.Send(msg)
					}
				}
			}
		}()
	} else {
		logger.With("kind", "연결").Warn("LS 앱키 없음, 실시간 뉴스·지수 미연결")
	}

	go func() {
		for _, msg := range fakeMessages(time.Now()) {
			p.Send(msg)
		}
	}()
	_, err = p.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil // Ctrl+C 로 컨텍스트가 취소된 정상 종료
	}
	return err
}
```

`lsSubscriptions` (`internal/tui/ls.go`) 에 JIF 두 건을 추가한다:

```go
var lsSubscriptions = []ls.Subscription{
	{TrCd: "NWS", TrKey: "NWS001"},
	{TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"},
	{TrCd: "JIF", TrKey: "1"}, {TrCd: "JIF", TrKey: "2"},
}
```
그리고 `internal/tui/ls_test.go` 의 `TestLSSubscriptions` 기대 목록에도 같은 두 건을 덧붙인다.

- [ ] **Step 9: fake.go 에서 로그 항목 삭제**

`fakeMessages` 에서 `LogMsg{...}` 블록을 지우고 주석을 이렇게 바꾼다:

```go
// fakeMessages 는 아직 실데이터가 없는 패널의 화면 확인용 가짜 데이터다.
// 7단계(관심종목), 8단계(보유종목)에서 실데이터로 바꾸며 해당 항목을 지우고, 다 지워지면 이 파일을 삭제한다.
```

- [ ] **Step 10: 테스트 통과 확인**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... -count=1 2>&1 | tail -6`
Expected: 전부 ok. tui 는 기존 + 4개(MarketStatus, FooterStatus, ToLogLines, WatchLog).

- [ ] **Step 11: 커밋 요청**

```bash
git add internal/tui
git commit -m "feat(tui): 로그 패널 파일 연동, 장 상태 표시, 파일 로거 배선"
```

---

### Task 4: 스펙 갱신과 수동 확인

**Files:**
- Modify: `docs/superpowers/specs/2026-09-13-backtest-design.md`

- [ ] **Step 1: 스펙 갱신**

10.1절 상단 행: "없으면 최근 20건을 5초마다 순환" 문구를 삭제하고 "새 뉴스 수신 즉시 교체. 뉴스가 없으면 `연결됨 · 뉴스 대기`" 로.
10.1절 하단 행: 내용을 "`[장 상태]` + 코스피·코스닥 현재 지수와 등락률. 장 상태는 시계 기준(`market.Status`), 그날 JIF 를 받았으면 그 값" 로.
10.3절: "TUI 로그 패널은 시작 시 마지막 200줄을 읽고, 1초마다 파일 크기를 확인해 늘어난 만큼 읽는다" 뒤에 "패키지 `internal/logfile`. 로그 파일을 열 수 없으면 TUI 는 로거를 버리고 로그 패널에 오류 한 줄만 보인다" 추가. 11절 마지막 항목의 "stderr로 대신 쓰고" 는 서브커맨드에만 해당한다고 명시.
10.2절 JIF 항목: "2단계 `ls-probe` 에서 구독해 실제 상태 코드를 확인하고" 를 "TUI 도 구독하며 `market.FromJIF` 코드표(11/21/31/41/51/52/61/62)로 하단 장 상태를 덮어쓴다. 표에 없는 코드는 로그만" 로.
3절 디렉터리에 `internal/logfile/`, `internal/market/` 추가.

- [ ] **Step 2: 화면 수동 확인 (사용자)**

```bash
go run ./cmd/trader
```

확인 항목:
1. 하단 줄이 ` [장마감] 코스피 …` 처럼 현재 시각에 맞는 상태로 시작 (주말이면 `[휴장]`).
2. 메뉴에서 로그 → 패널에 `[연결] ls websocket 연결 subs=5` 줄이 보인다 (앱키 없으면 `LS 앱키 없음 …`).
3. 다른 터미널에서 `echo '{"time":"'$(date -Iseconds)'","level":"INFO","msg":"수동 테스트","kind":"수집"}' >> data/trader.log` 를 치면 1초 안에 로그 패널 맨 위에 현재 시각으로 나타난다.
4. `q` 로 종료 후 `tail -3 data/trader.log` 에 JSON 줄들이 있다.
5. (다음 영업일) 09:00, 15:20, 15:30 근처에 켜 두면 로그에 `kospi 장운영 21 status=장중 known=true` 같은 줄이 찍히고 하단 상태가 바뀐다. 코드가 표와 다르면 `market.go` 의 `jifStatus` 를 관측값으로 고친다.

- [ ] **Step 3: 커밋 요청**

```bash
git add docs
git commit -m "docs: 3단계 반영 — 로그 파일 패키지, 장 상태 표시, 뉴스 순환 삭제"
```

---

## 완료 기준

- `go test ./...` 전부 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- `trader` 실행 시 로그 패널이 `data/trader.log` 를 따라가고, 하단에 장 상태가 보임
- 다음 단계(4단계 SQLite)는 이 단계 산출물을 소비하지 않는다. 6단계 `universe`/`fetch` 는 `logfile.Open` 으로 같은 파일에 `kind=수집` 로그를 쓴다. 8단계는 `run.go` 의 `logger` 를 잔고 폴링에 넘긴다.
