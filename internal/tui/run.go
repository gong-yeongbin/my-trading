package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gong-yeongbin/my-trading/internal/app"
	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/logfile"
	"github.com/gong-yeongbin/my-trading/internal/ls"
	"github.com/gong-yeongbin/my-trading/internal/market"
	"github.com/gong-yeongbin/my-trading/internal/settings"
)

// Run 은 전체 화면 TUI 를 띄우고 종료될 때까지 막는다.
// 로그는 cfg.Log.File 에 쓰고 로그 패널이 그 파일을 따라간다. LS 앱키가 있으면 실시간 뉴스·지수·장운영정보를 구독한다.
// 한투 앱키가 있으면 일봉을 자동 수집한다.
func Run(ctx context.Context, cfg *config.Config) error {
	// 취소 가능한 자식 컨텍스트: Run 안에서 시작하는 고루틴들이 q 로 TUI 종료 시 함께 멈추도록.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := New()
	m.saver = func(v settings.Values) error { return settings.Save(".env", "config.yaml", v) }
	refresh := make(chan struct{}, 1) // 보유종목 새로고침(r). 버퍼 1: 연타해도 한 번
	m.refresh = func() {
		select {
		case refresh <- struct{}{}:
		default:
		}
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	go func() {
		v, err := settings.Load(".env", "config.yaml")
		p.Send(SettingsMsg{Values: v, Err: err})
	}()

	// 로그 파일. 못 열면 TUI 는 계속 뜨고 로그 패널에 오류 한 줄만 보인다 (스펙 11).
	logger, closer, err := logfile.Open(cfg.Log.File)
	if err != nil {
		logger = slog.New(slog.DiscardHandler)
		initial := []logfile.Line{{Time: time.Now(), Level: "ERROR", Kind: "오류", Msg: "로그 파일 열기 실패: " + err.Error()}}
		go p.Send(LogMsg{Lines: toLogLines(initial)})
	} else {
		initial, size, _ := logfile.Tail(cfg.Log.File, logKeep)
		go watchLog(ctx, logfile.NewReader(cfg.Log.File, size), initial, time.Second, p.Send)
	}

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
						indexLog.Info(fmt.Sprintf("%s 장운영 %s", ms.Market, ms.Code), "status", status.String(), "known", known)
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

	// 저장소: 관심종목 계산·자동 수집이 함께 쓴다. 앱키 유무와 무관하게 연다.
	store, storeErr := data.Open(cfg.DBPath)
	if storeErr != nil {
		logger.With("kind", "지수").Error("DB 열기 실패 — 관심종목·자동 수집 비활성", "err", storeErr)
	} else {
		defer store.Close()
		screenLog := logger.With("kind", "지수")
		go runScreen(ctx, cfg, store, screenLog, p.Send)

		// 자동 일봉 수집: 앱키가 있으면 켤 때 따라잡고, 매일 daily_at 에 돈다.
		fetchLog := logger.With("kind", "수집")
		if err := cfg.RequireMarketKey(); err != nil {
			fetchLog.Warn("자동 수집 비활성: " + err.Error())
		} else {
			client := kis.New(cfg.KIS.MarketBaseURL(), cfg.KIS.Market.AppKey, cfg.KIS.Market.AppSecret, cfg.KIS.TokenCache, cfg.KIS.MarketRPS())
			stale := func() (bool, error) {
				last, has, err := store.LastIndexBarDate(ctx, "kospi")
				if err != nil {
					fetchLog.Warn("따라잡기 판정 실패", "err", err)
					return false, err
				}
				return app.Stale(last, has, time.Now()), nil
			}
			next := func(now time.Time) (time.Time, error) { return app.NextRun(now, cfg.Fetch.DailyAt) }
			go fetchLoop(ctx, time.Now, stale, next, func(c context.Context) {
				runFetchOnce(c, cfg, store, client, fetchLog, p.Send, func() { runScreen(c, cfg, store, screenLog, p.Send) })
			})
		}

		// 보유종목: 매매 서버(trade_env)의 계좌를 켤 때·장전 08:59·r 키에 조회한다.
		holdLog := logger.With("kind", "매매")
		if err := cfg.RequireTradeKey(); err != nil {
			holdLog.Warn("보유종목 비활성: " + err.Error())
			go p.Send(HoldingsMsg{Connected: false})
		} else if acct, err := kis.ParseAccount(cfg.KIS.Trade.Account); err != nil {
			holdLog.Warn("보유종목 비활성: " + err.Error() + " — 설정 메뉴에서 계좌번호 입력")
			go p.Send(HoldingsMsg{Connected: false})
		} else {
			trade := kis.New(cfg.KIS.TradeBaseURL(), cfg.KIS.Trade.AppKey, cfg.KIS.Trade.AppSecret, cfg.KIS.TradeTokenCache, cfg.KIS.TradeRPS())
			demo := cfg.KIS.TradeEnv != "real"
			fetch := func(c context.Context) (HoldingsMsg, error) {
				b, err := trade.Balance(c, acct, demo)
				if err != nil {
					return HoldingsMsg{}, err
				}
				syms, _ := store.ListSymbols(c)
				mk := map[string]string{}
				for _, s := range syms {
					mk[s.Code] = s.Market
				}
				return holdingsMsgFrom(b, func(code string) string { return mk[code] }), nil
			}
			go holdingsLoop(ctx, time.Minute, time.Now, refresh, fetch, p.Send, holdLog)
		}
	}

	_, err = p.Run()
	cancel()
	if closer != nil {
		closer.Close()
	}
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil // Ctrl+C 로 컨텍스트가 취소된 정상 종료
	}
	return err
}
