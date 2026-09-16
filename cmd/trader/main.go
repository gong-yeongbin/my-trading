// trader 는 종가 베팅 운영 도구의 CLI 진입점이다.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/ls"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

const usage = `사용법:
  trader                 TUI
  trader ls-probe [초]   LS 실시간 이벤트를 N초(기본 30) 동안 출력 (연결 확인용)
  trader universe        (6단계 계획에서 구현)
  trader fetch           (6단계 계획에서 구현)
  trader watch           (7단계 계획에서 구현)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf(".env: %w", err)
	}
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return tui.Run(ctx, cfg)
	}
	switch args[0] {
	case "ls-probe":
		return runLSProbe(ctx, cfg, args[1:])
	case "universe", "fetch", "watch":
		return fmt.Errorf("%s 는 아직 구현되지 않았습니다", args[0])
	default:
		fmt.Print(usage)
		return fmt.Errorf("알 수 없는 명령 %q", args[0])
	}
}

// runLSProbe 는 LS 웹소켓에 연결해 받은 이벤트를 그대로 찍는다. 로그는 stderr, 이벤트는 stdout.
func runLSProbe(ctx context.Context, cfg *config.Config, args []string) error {
	if !cfg.LS.HasAppKey() {
		return fmt.Errorf("LS_APP_KEY / LS_APP_SECRET 환경변수가 없습니다 (.env 파일을 확인하세요)")
	}
	secs := 30
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("초는 양의 정수여야 합니다: %q", args[0])
		}
		secs = n
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(secs)*time.Second)
	defer cancel()

	client := ls.New(ls.Config{
		BaseURL: cfg.LS.BaseURL, WSURL: cfg.LS.WSURL,
		AppKey: cfg.LS.AppKey, AppSecret: cfg.LS.AppSecret, TokenCache: cfg.LS.TokenCache,
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// TUI 와 같은 구독 목록(뉴스·업종지수·장운영정보 JIF)을 그대로 써서 실제 수신 값을 본다.
	subs := []ls.Subscription{
		{TrCd: "NWS", TrKey: "NWS001"},
		{TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"},
		{TrCd: "JIF", TrKey: "1"}, {TrCd: "JIF", TrKey: "2"},
	}
	events := make(chan ls.Event, 64)
	go client.Run(ctx, subs, events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			fmt.Printf("%s %T %+v\n", time.Now().Format("15:04:05"), ev, ev)
		}
	}
}
