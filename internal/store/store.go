package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"paypay-nisa-analyzer/internal/domain"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

var openSQLite = sql.Open

func Open(path string) (*Store, error) {
	db, err := openSQLite("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS funds (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, manager TEXT NOT NULL DEFAULT '',
  nisa_tsumitate INTEGER NOT NULL DEFAULT 0, nisa_growth INTEGER NOT NULL DEFAULT 0,
  paypay_url TEXT NOT NULL DEFAULT '', history_url TEXT NOT NULL DEFAULT '', refreshed_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS prices (
  fund_id TEXT NOT NULL REFERENCES funds(id) ON DELETE CASCADE, date TEXT NOT NULL,
  nav REAL NOT NULL CHECK(nav > 0), source_url TEXT NOT NULL, PRIMARY KEY(fund_id, date)
);
CREATE TABLE IF NOT EXISTS insights (
  id INTEGER PRIMARY KEY AUTOINCREMENT, fund_id TEXT NOT NULL REFERENCES funds(id) ON DELETE CASCADE,
  publisher TEXT NOT NULL, title TEXT NOT NULL, summary TEXT NOT NULL, source_url TEXT NOT NULL,
  published_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_funds_name ON funds(name);
CREATE INDEX IF NOT EXISTS idx_prices_fund_date ON prices(fund_id, date);
CREATE INDEX IF NOT EXISTS idx_insights_fund_date ON insights(fund_id, published_at DESC);`)
	return err
}

func (s *Store) UpsertFunds(funds []domain.Fund) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, f := range funds {
		if f.ID == "" || f.Name == "" {
			return errors.New("基金資料缺少 ID 或名稱")
		}
		if _, err := tx.Exec(`INSERT INTO funds(id,name,manager,nisa_tsumitate,nisa_growth,paypay_url,history_url,refreshed_at)
VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,manager=excluded.manager,nisa_tsumitate=excluded.nisa_tsumitate,nisa_growth=excluded.nisa_growth,paypay_url=excluded.paypay_url,history_url=excluded.history_url,refreshed_at=excluded.refreshed_at`, f.ID, f.Name, f.Manager, boolInt(f.NISATsumitate), boolInt(f.NISAGrowth), f.PayPayURL, f.HistoryURL, f.RefreshedAt.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) FindFunds(query string, limit int) ([]domain.Fund, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id,name,manager,nisa_tsumitate,nisa_growth,paypay_url,history_url,refreshed_at FROM funds WHERE name LIKE ? ORDER BY name LIMIT ?`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Fund{}
	for rows.Next() {
		f, err := scanFund(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (s *Store) LastFundRefresh() (time.Time, error) {
	var value sql.NullString
	if err := s.db.QueryRow(`SELECT MAX(refreshed_at) FROM funds`).Scan(&value); err != nil {
		return time.Time{}, err
	}
	if !value.Valid || value.String == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value.String)
}

func (s *Store) Fund(id string) (domain.Fund, error) {
	row := s.db.QueryRow(`SELECT id,name,manager,nisa_tsumitate,nisa_growth,paypay_url,history_url,refreshed_at FROM funds WHERE id=?`, id)
	f, err := scanFund(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Fund{}, fmt.Errorf("找不到基金：%s", id)
	}
	return f, err
}

func (s *Store) ReplacePrices(fundID string, points []domain.PricePoint) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM prices WHERE fund_id=?`, fundID); err != nil {
		return err
	}
	for _, p := range points {
		if p.NAV <= 0 || p.Date.IsZero() {
			return errors.New("歷史淨值無效")
		}
		if _, err = tx.Exec(`INSERT INTO prices(fund_id,date,nav,source_url) VALUES(?,?,?,?)`, fundID, p.Date.UTC().Format("2006-01-02"), p.NAV, p.SourceURL); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Prices(fundID string) ([]domain.PricePoint, error) {
	rows, err := s.db.Query(`SELECT fund_id,date,nav,source_url FROM prices WHERE fund_id=? ORDER BY date`, fundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []domain.PricePoint{}
	for rows.Next() {
		var p domain.PricePoint
		var date string
		if err := rows.Scan(&p.FundID, &date, &p.NAV, &p.SourceURL); err != nil {
			return nil, err
		}
		p.Date, err = time.Parse("2006-01-02", date)
		if err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func (s *Store) Insights(fundID string) ([]domain.Insight, error) {
	rows, err := s.db.Query(`SELECT id,fund_id,publisher,title,summary,source_url,published_at FROM insights WHERE fund_id=? ORDER BY published_at DESC LIMIT 10`, fundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Insight{}
	for rows.Next() {
		var in domain.Insight
		var published string
		if err := rows.Scan(&in.ID, &in.FundID, &in.Publisher, &in.Title, &in.Summary, &in.SourceURL, &published); err != nil {
			return nil, err
		}
		in.PublishedAt, err = time.Parse(time.RFC3339, published)
		if err != nil {
			return nil, err
		}
		result = append(result, in)
	}
	return result, rows.Err()
}

type fundScanner interface{ Scan(...any) error }

func scanFund(row fundScanner) (domain.Fund, error) {
	var f domain.Fund
	var tsumitate, growth int
	var refreshed string
	err := row.Scan(&f.ID, &f.Name, &f.Manager, &tsumitate, &growth, &f.PayPayURL, &f.HistoryURL, &refreshed)
	if err != nil {
		return f, err
	}
	f.NISATsumitate, f.NISAGrowth = tsumitate == 1, growth == 1
	f.RefreshedAt, err = time.Parse(time.RFC3339, refreshed)
	return f, err
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
