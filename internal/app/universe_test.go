package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
)

type fakeMaster struct {
	rows map[string][]kis.MasterRow
	err  error
}

func (f fakeMaster) DownloadMaster(_ context.Context, market string) ([]kis.MasterRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[market], nil
}

func testConfig() *config.Config {
	return &config.Config{
		Universe: config.UniverseConfig{Markets: []string{"kospi", "kosdaq"}, GroupCodes: []string{"ST"},
			ExcludeFlags: []string{"거래정지", "관리종목", "SPAC"}},
	}
}

func openStore(t *testing.T) *data.SQLiteStore {
	t.Helper()
	s, err := data.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunUniverseFilters(t *testing.T) {
	src := fakeMaster{rows: map[string][]kis.MasterRow{
		"kospi": {
			{Code: "005930", Name: "삼성전자", Market: "kospi", GroupCode: "ST", WarningCode: "00", OverheatCode: "0"},
			{Code: "069500", Name: "KODEX 200", Market: "kospi", GroupCode: "EF", WarningCode: "00", OverheatCode: "0"},
			{Code: "000040", Name: "KR모터스", Market: "kospi", GroupCode: "ST", Managed: true, WarningCode: "00", OverheatCode: "0"},
		},
		"kosdaq": {
			{Code: "247540", Name: "에코프로비엠", Market: "kosdaq", GroupCode: "ST", WarningCode: "02", OverheatCode: "0"},
			{Code: "0004Y0", Name: "디비금융제14호스팩", Market: "kosdaq", GroupCode: "ST", Spac: true, WarningCode: "00", OverheatCode: "0"},
		},
	}}
	store := openStore(t)
	// 이전 실행에서 저장된 종목: KR모터스는 이번에 관리종목이라 삭제, 옛날종목은 마스터에 없으니 보존(봉 이력)
	store.UpsertSymbols(context.Background(), []data.Symbol{{Code: "000040", Name: "KR모터스", Market: "kospi"}, {Code: "999999", Name: "옛날종목", Market: "kospi"}})
	res, err := RunUniverse(context.Background(), testConfig(), store, src)
	if err != nil {
		t.Fatal(err)
	}
	syms, _ := store.ListSymbols(context.Background())
	if len(syms) != 3 || syms[0].Code != "005930" || syms[1].Code != "247540" || syms[1].Market != "kosdaq" || syms[2].Code != "999999" {
		t.Errorf("symbols = %+v", syms)
	}
	if res.Downloaded != 5 || res.Kept != 2 || res.Dropped != 1 || res.ByMarket["kospi"] != 1 || res.ByMarket["kosdaq"] != 1 {
		t.Errorf("result = %+v", res)
	}
}

func TestRunUniverseRejectsUnknownFlagBeforeDownload(t *testing.T) {
	cfg := testConfig()
	cfg.Universe.ExcludeFlags = []string{"없는플래그"}
	src := fakeMaster{err: errors.New("must not be called")}
	if _, err := RunUniverse(context.Background(), cfg, openStore(t), src); err == nil || err.Error() == "must not be called" {
		t.Errorf("expected flag validation error before download, got %v", err)
	}
}

func TestRunUniversePropagatesDownloadError(t *testing.T) {
	src := fakeMaster{err: errors.New("network down")}
	if _, err := RunUniverse(context.Background(), testConfig(), openStore(t), src); err == nil {
		t.Error("expected error")
	}
}
