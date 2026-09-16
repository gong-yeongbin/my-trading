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
	// fn 이 패닉해도 트랜잭션을 풀어 단일 연결(SetMaxOpenConns(1))이 영원히 잠기지 않게 한다.
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
