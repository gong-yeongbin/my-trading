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
