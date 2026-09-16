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
