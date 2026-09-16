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

	var first LogMsg
	select {
	case m := <-msgs:
		first = m.(LogMsg)
	case <-time.After(2 * time.Second):
		t.Fatal("no initial LogMsg")
	}
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
