# 4단계: SQLite 저장소 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 종목·일봉·지수 일봉 타입과 `data.Store` 인터페이스, SQLite 구현을 만든다. 이후 단계(한투 수집, 선별, TUI)가 전부 이 저장소를 읽고 쓴다.

**Architecture:** `data`(타입, Store 인터페이스, SQLiteStore). 1단계의 `config.Config.DBPath`를 소비한다.

**Tech Stack:** Go 1.27, `modernc.org/sqlite`(CGO 없음). 테스트는 표준 `testing`만 사용.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (6절, 12절의 data 항목)

**선행:** 1단계(모듈·설정·TUI 뼈대) 완료.

## Global Constraints

- 모듈 경로: `github.com/gong-yeongbin/my-trading`
- 외부 의존성은 위 3개로 제한. 테스트 프레임워크(testify 등) 추가 금지.
- 가격은 `int64` 원 단위, 지수는 `float64`. 날짜는 `YYYY-MM-DD` 문자열로 저장하고 `time.Time`은 KST 자정.
- 한투 실서버는 테스트에서 호출하지 않는다. 마지막 수동 확인에서만 호출.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/token.json`은 읽기 금지. 값이 필요하면 프로그램이 읽게 하고 Claude는 열지 않는다.
- 파일 편집 후 `gofmt -l .` 결과가 비어 있어야 한다.

---


## 파일 구조

```
internal/data/types.go        Symbol, Bar, IndexBar, KST, 날짜 헬퍼
internal/data/store.go        Store 인터페이스
internal/data/sqlite.go       SQLiteStore
internal/data/sqlite_test.go
```

---

### Task 1: 데이터 타입과 SQLite 저장소

**Files:**
- Create: `internal/data/types.go`, `internal/data/store.go`, `internal/data/sqlite.go`
- Test: `internal/data/sqlite_test.go`

**Interfaces:**
- Produces: `data.Symbol{Code, Name, Market string}`, `data.Bar{Date time.Time; Open, High, Low, Close, Volume int64}`, `data.IndexBar{Date time.Time; Open, High, Low, Close float64}`, `data.KST`, `data.ParseDate(s string) (time.Time, error)`, `data.FormatDate(t time.Time) string`, `data.Store` 인터페이스, `data.Open(path string) (*SQLiteStore, error)`, `(*SQLiteStore).Close() error`.

- [ ] **Step 1: 의존성 추가**

```bash
go get modernc.org/sqlite@latest
```

- [ ] **Step 2: 타입과 인터페이스 작성 (테스트가 컴파일되도록 먼저)**

`internal/data/types.go`:

```go
// Package data 는 시세 타입과 로컬 저장소를 정의한다.
package data

import "time"

// KST 는 모든 날짜의 기준 시간대다. 봉의 Date 는 KST 자정이다.
var KST = time.FixedZone("KST", 9*3600)

type Symbol struct {
	Code   string // 6자리 단축코드
	Name   string
	Market string // kospi | kosdaq
}

type Bar struct {
	Date   time.Time
	Open   int64
	High   int64
	Low    int64
	Close  int64
	Volume int64
}

type IndexBar struct {
	Date  time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
}

const dateLayout = "2006-01-02"

func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, KST)
}

func FormatDate(t time.Time) string {
	return t.In(KST).Format(dateLayout)
}

// Date 는 y-m-d 를 KST 자정으로 만든다. 테스트와 CLI 에서 쓴다.
func Date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, KST)
}
```

`internal/data/store.go`:

```go
package data

import (
	"context"
	"time"
)

type Store interface {
	UpsertSymbols(ctx context.Context, syms []Symbol) error
	ListSymbols(ctx context.Context) ([]Symbol, error)

	LastBarDate(ctx context.Context, code string) (time.Time, bool, error)
	UpsertBars(ctx context.Context, code string, bars []Bar) error
	LoadBars(ctx context.Context, code string, from, to time.Time) ([]Bar, error)

	LastIndexBarDate(ctx context.Context, market string) (time.Time, bool, error)
	UpsertIndexBars(ctx context.Context, market string, bars []IndexBar) error
	LoadIndexBars(ctx context.Context, market string, from, to time.Time) ([]IndexBar, error)
}
```

- [ ] **Step 3: 실패하는 테스트 작성**

`internal/data/sqlite_test.go`:

```go
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
```

- [ ] **Step 4: 테스트 실패 확인**

Run: `go test ./internal/data/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: Open`, `SQLiteStore`)

- [ ] **Step 5: SQLite 구현**

`internal/data/sqlite.go`:

```go
package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS symbols (
		code   TEXT PRIMARY KEY,
		name   TEXT NOT NULL,
		market TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS bars (
		code   TEXT NOT NULL,
		date   TEXT NOT NULL,
		open   INTEGER NOT NULL,
		high   INTEGER NOT NULL,
		low    INTEGER NOT NULL,
		close  INTEGER NOT NULL,
		volume INTEGER NOT NULL,
		PRIMARY KEY (code, date)
	)`,
	`CREATE TABLE IF NOT EXISTS index_bars (
		market TEXT NOT NULL,
		date   TEXT NOT NULL,
		open   REAL NOT NULL,
		high   REAL NOT NULL,
		low    REAL NOT NULL,
		close  REAL NOT NULL,
		PRIMARY KEY (market, date)
	)`,
}

type SQLiteStore struct {
	db *sql.DB
}

var _ Store = (*SQLiteStore)(nil)

// Open 은 파일을 열고(없으면 디렉터리와 함께 생성) 스키마를 보장한다.
func Open(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("data: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("data: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range append([]string{`PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`}, schema...) {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("data: schema: %w", err)
		}
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) UpsertSymbols(ctx context.Context, syms []Symbol) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO symbols(code, name, market) VALUES(?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET name = excluded.name, market = excluded.market`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, sym := range syms {
			if _, err := stmt.ExecContext(ctx, sym.Code, sym.Name, sym.Market); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLiteStore) ListSymbols(ctx context.Context) ([]Symbol, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, name, market FROM symbols ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Symbol
	for rows.Next() {
		var sym Symbol
		if err := rows.Scan(&sym.Code, &sym.Name, &sym.Market); err != nil {
			return nil, err
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) LastBarDate(ctx context.Context, code string) (time.Time, bool, error) {
	return s.lastDate(ctx, `SELECT MAX(date) FROM bars WHERE code = ?`, code)
}

func (s *SQLiteStore) UpsertBars(ctx context.Context, code string, bars []Bar) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO bars(code, date, open, high, low, close, volume) VALUES(?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(code, date) DO UPDATE SET open = excluded.open, high = excluded.high, low = excluded.low, close = excluded.close, volume = excluded.volume`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, b := range bars {
			if _, err := stmt.ExecContext(ctx, code, FormatDate(b.Date), b.Open, b.High, b.Low, b.Close, b.Volume); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLiteStore) LoadBars(ctx context.Context, code string, from, to time.Time) ([]Bar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date, open, high, low, close, volume FROM bars
		WHERE code = ? AND date >= ? AND date <= ? ORDER BY date`, code, FormatDate(from), FormatDate(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bar
	for rows.Next() {
		var b Bar
		var d string
		if err := rows.Scan(&d, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume); err != nil {
			return nil, err
		}
		if b.Date, err = ParseDate(d); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) LastIndexBarDate(ctx context.Context, market string) (time.Time, bool, error) {
	return s.lastDate(ctx, `SELECT MAX(date) FROM index_bars WHERE market = ?`, market)
}

func (s *SQLiteStore) UpsertIndexBars(ctx context.Context, market string, bars []IndexBar) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO index_bars(market, date, open, high, low, close) VALUES(?, ?, ?, ?, ?, ?)
			ON CONFLICT(market, date) DO UPDATE SET open = excluded.open, high = excluded.high, low = excluded.low, close = excluded.close`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, b := range bars {
			if _, err := stmt.ExecContext(ctx, market, FormatDate(b.Date), b.Open, b.High, b.Low, b.Close); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLiteStore) LoadIndexBars(ctx context.Context, market string, from, to time.Time) ([]IndexBar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date, open, high, low, close FROM index_bars
		WHERE market = ? AND date >= ? AND date <= ? ORDER BY date`, market, FormatDate(from), FormatDate(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexBar
	for rows.Next() {
		var b IndexBar
		var d string
		if err := rows.Scan(&d, &b.Open, &b.High, &b.Low, &b.Close); err != nil {
			return nil, err
		}
		if b.Date, err = ParseDate(d); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) lastDate(ctx context.Context, query string, arg string) (time.Time, bool, error) {
	var d sql.NullString
	if err := s.db.QueryRowContext(ctx, query, arg).Scan(&d); err != nil {
		return time.Time{}, false, err
	}
	if !d.Valid {
		return time.Time{}, false, nil
	}
	t, err := ParseDate(d.String)
	return t, err == nil, err
}

func (s *SQLiteStore) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
```

- [ ] **Step 6: 테스트 통과 확인**

Run: `go test ./internal/data/ -v 2>&1 | tail -12`
Expected: 4개 테스트 PASS

- [ ] **Step 7: 커밋 요청**

```bash
git add go.mod go.sum internal/data
git commit -m "feat(data): 시세 타입과 SQLite 저장소"
```

---


## 완료 기준

- `go test ./internal/data/...` 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- 다음 단계(5단계 한투 클라이언트)는 `data.Bar`, `data.IndexBar`, `data.KST`, `data.ParseDate`를 그대로 소비한다
