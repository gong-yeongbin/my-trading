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
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
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
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, sb.String())
	return Line{Time: t, Level: level, Kind: kind, Msg: clean}, true
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
	end := bytes.LastIndexByte(b, '\n') + 1
	lines := parseLines(b[:end])
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, int64(end), nil
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
