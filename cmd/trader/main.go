// trader 는 종가 베팅 운영 도구의 CLI 진입점이다.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/app"
	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/logfile"
	"github.com/gong-yeongbin/my-trading/internal/ls"
	"github.com/gong-yeongbin/my-trading/internal/screener"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

const usage = `사용법:
  trader                 TUI
  trader ls-probe [초]   LS 실시간 이벤트를 N초(기본 30) 동안 출력 (연결 확인용)
  trader universe        마스터 파일로 종목 목록 갱신
  trader fetch [--from YYYY-MM-DD]   지수·종목 일봉 증분 수집
  trader watch           전일 데이터 기준 관심 종목 표 출력
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
	case "universe":
		return runUniverse(ctx, cfg)
	case "fetch":
		return runFetch(ctx, cfg, args[1:])
	case "watch":
		return runWatch(ctx, cfg)
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

func newClient(cfg *config.Config) (*kis.Client, error) {
	if err := cfg.RequireMarketKey(); err != nil {
		return nil, err
	}
	return kis.New(cfg.KIS.MarketBaseURL(), cfg.KIS.Market.AppKey, cfg.KIS.Market.AppSecret, cfg.KIS.TokenCache, cfg.KIS.MarketRPS()), nil
}

// openLogger 는 로그 파일을 연다. 열 수 없으면 stderr 로 대신 기록하며 경고를 찍는다 (spec §11).
func openLogger(cfg *config.Config) (*slog.Logger, func()) {
	logger, closer, err := logfile.Open(cfg.Log.File)
	if err != nil {
		fmt.Fprintln(os.Stderr, "경고: 로그 파일을 열 수 없어 표준 오류로 대신 기록합니다:", err)
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() {}
	}
	return logger, func() { closer.Close() }
}

func runUniverse(ctx context.Context, cfg *config.Config) error {
	logger, done := openLogger(cfg)
	defer done()
	logger = logger.With("kind", "수집")

	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	store, err := data.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	res, err := app.RunUniverse(ctx, cfg, store, client)
	if err != nil {
		logger.Error("유니버스 갱신 실패", "err", err)
		return err
	}
	fmt.Printf("마스터 %d 종목 중 %d 종목 저장, %d 종목 제외 삭제", res.Downloaded, res.Kept, res.Dropped)
	logArgs := []any{"downloaded", res.Downloaded, "kept", res.Kept}
	for _, m := range cfg.Universe.Markets {
		fmt.Printf("  %s %d", m, res.ByMarket[m])
		logArgs = append(logArgs, m, res.ByMarket[m])
	}
	fmt.Println()
	logger.Info("유니버스 갱신", logArgs...)
	return nil
}

func runFetch(ctx context.Context, cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	fromStr := fs.String("from", "", "저장된 봉이 없을 때의 시작일 (YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var opts app.FetchOptions
	if *fromStr != "" {
		from, err := data.ParseDate(*fromStr)
		if err != nil {
			return fmt.Errorf("--from: %w", err)
		}
		opts.From = from
	}

	logger, done := openLogger(cfg)
	defer done()
	logger = logger.With("kind", "수집")

	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	store, err := data.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	logger.Info("일봉 수집 시작", "server", "real", "from", *fromStr)

	started := time.Now()
	fmt.Fprint(os.Stderr, "[real] 지수 일봉 수집 중...\n")
	res, err := app.RunFetch(ctx, cfg, store, client, opts, func(p app.FetchProgress) {
		if p.Err != nil {
			fmt.Fprintf(os.Stderr, "\r[%d/%d] %s 실패: %v\n", p.Done, p.Total, p.Code, p.Err)
			logger.Warn("종목 수집 실패", "code", p.Code, "err", p.Err)
			return
		}
		fmt.Fprintf(os.Stderr, "\r[%d/%d] %s", p.Done, p.Total, p.Code)
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Printf("중단됨: 종목 %d 성공, 봉 %d 저장, 실패 %d\n", res.Symbols, res.Bars, len(res.Failures))
			logger.Warn("일봉 수집 중단 (사용자)", "symbols", res.Symbols, "bars", res.Bars, "failed", len(res.Failures))
			return err
		}
		logger.Error("일봉 수집 중단", "err", err)
		return err
	}
	elapsed := time.Since(started).Round(time.Second)
	fmt.Printf("종목 %d 성공, 최신 %d, 봉 %d 저장, 실패 %d, 소요 %s\n", res.Symbols, res.UpToDate, res.Bars, len(res.Failures), elapsed)
	for _, f := range res.Failures {
		fmt.Printf("  실패 %s: %v\n", f.Code, f.Err)
	}
	logger.Info("일봉 수집 완료", "symbols", res.Symbols, "bars", res.Bars, "failed", len(res.Failures), "elapsed", elapsed.String())
	return nil
}

// runWatch 는 저장소만 읽어 관심종목 표를 출력한다. 네트워크를 쓰지 않는다.
func runWatch(ctx context.Context, cfg *config.Config) error {
	store, err := data.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	res, err := screener.Run(ctx, cfg, store)
	if err != nil {
		return err
	}
	if res.AsOf.IsZero() {
		fmt.Println("지수 봉이 없습니다. 먼저 fetch 를 실행하세요.")
		return nil
	}
	fmt.Printf("기준일 %s  코스피 %s · 코스닥 %s  관심종목 %d개\n", res.AsOf.Format("2006-01-02"), res.Filter["kospi"], res.Filter["kosdaq"], len(res.Items))
	fmt.Printf("%-8s %-16s %-6s %10s %10s %8s %12s %12s\n", "코드", "종목명", "시장", "전일종가", "필요종가", "필요상승", "필요거래량", "거래대금(억)")
	// %-16s 는 한글 폭을 못 맞추지만 CLI 는 참고용이라 그대로 둔다.
	for _, it := range res.Items {
		fmt.Printf("%-8s %-16s %-6s %10d %10d %7.1f%% %12d %12d\n", it.Code, it.Name, it.Market, it.PrevClose, it.MinClose, it.MinChangePct*100, it.MinVolume, it.AvgTurnover/100_000_000)
	}
	return nil
}
