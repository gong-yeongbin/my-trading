package data

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sub", "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSymbolsUpsert(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.UpsertSymbols(ctx, []Symbol{{"005930", "삼성전자", "kospi"}, {"247540", "에코프로비엠", "kosdaq"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSymbols(ctx, []Symbol{{"005930", "삼성전자(개명)", "kospi"}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListSymbols(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Code != "005930" || got[0].Name != "삼성전자(개명)" || got[1].Code != "247540" {
		t.Errorf("ListSymbols = %+v", got)
	}
}

func TestBarsRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, ok, err := s.LastBarDate(ctx, "005930"); err != nil || ok {
		t.Fatalf("LastBarDate on empty: ok=%v err=%v", ok, err)
	}
	bars := []Bar{
		{Date: Date(2024, 9, 3), Open: 74100, High: 74300, Low: 72500, Close: 72500, Volume: 16314599},
		{Date: Date(2024, 9, 2), Open: 74500, High: 74700, Low: 73500, Close: 74400, Volume: 12641376},
	}
	if err := s.UpsertBars(ctx, "005930", bars); err != nil {
		t.Fatal(err)
	}
	// 같은 날짜 다시 넣으면 덮어쓴다
	if err := s.UpsertBars(ctx, "005930", []Bar{{Date: Date(2024, 9, 3), Open: 1, High: 2, Low: 1, Close: 2, Volume: 3}}); err != nil {
		t.Fatal(err)
	}
	last, ok, err := s.LastBarDate(ctx, "005930")
	if err != nil || !ok || !last.Equal(Date(2024, 9, 3)) {
		t.Fatalf("LastBarDate = %v ok=%v err=%v", last, ok, err)
	}
	got, err := s.LoadBars(ctx, "005930", Date(2024, 9, 1), Date(2024, 9, 30))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Date.Equal(Date(2024, 9, 2)) || got[1].Close != 2 {
		t.Errorf("LoadBars = %+v", got)
	}
	if got[0].Date.Location().String() != "KST" {
		t.Errorf("loaded date location = %v", got[0].Date.Location())
	}
	narrow, _ := s.LoadBars(ctx, "005930", Date(2024, 9, 3), Date(2024, 9, 3))
	if len(narrow) != 1 {
		t.Errorf("range filter: got %d bars", len(narrow))
	}
	if other, _ := s.LoadBars(ctx, "000000", Date(2024, 1, 1), Date(2024, 12, 31)); len(other) != 0 {
		t.Errorf("other code should be empty")
	}
}

func TestIndexBarsRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, ok, _ := s.LastIndexBarDate(ctx, "kospi"); ok {
		t.Fatal("expected no index bars")
	}
	if err := s.UpsertIndexBars(ctx, "kospi", []IndexBar{
		{Date: Date(2024, 9, 2), Open: 2650.1, High: 2660.2, Low: 2640.3, Close: 2655.4},
		{Date: Date(2024, 9, 3), Open: 2655.4, High: 2670.0, Low: 2600.0, Close: 2610.5},
	}); err != nil {
		t.Fatal(err)
	}
	last, ok, err := s.LastIndexBarDate(ctx, "kospi")
	if err != nil || !ok || !last.Equal(Date(2024, 9, 3)) {
		t.Fatalf("LastIndexBarDate = %v ok=%v err=%v", last, ok, err)
	}
	got, err := s.LoadIndexBars(ctx, "kospi", Date(2024, 9, 1), Date(2024, 9, 30))
	if err != nil || len(got) != 2 || got[1].Close != 2610.5 {
		t.Errorf("LoadIndexBars = %+v err=%v", got, err)
	}
	if kq, _ := s.LoadIndexBars(ctx, "kosdaq", Date(2024, 9, 1), Date(2024, 9, 30)); len(kq) != 0 {
		t.Errorf("kosdaq should be empty")
	}
}

func TestDateHelpers(t *testing.T) {
	d, err := ParseDate("2024-09-02")
	if err != nil || FormatDate(d) != "2024-09-02" || d.Hour() != 0 {
		t.Errorf("ParseDate/FormatDate: %v %v", d, err)
	}
	if _, err := ParseDate("20240902"); err == nil {
		t.Error("expected parse error")
	}
	if d.Sub(time.Date(2024, 9, 1, 15, 0, 0, 0, time.UTC)) != 0 {
		t.Errorf("KST midnight should equal previous day 15:00 UTC")
	}
}
