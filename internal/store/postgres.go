package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paypay-nisa-analyzer/internal/domain"
)

// PostgresStore 是正式環境的資料庫實作，透過 PostgreSQL 標準協定連線，
// 可使用 Supabase 提供的連線字串。
type PostgresStore struct{ pool *pgxpool.Pool }

var _ Repository = (*PostgresStore)(nil)

func OpenPostgres(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	// 限制連線數，避免長駐 API 超出 Supabase Session Pooler 的可用連線額度。
	config.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

func (s *PostgresStore) UpsertFunds(funds []domain.Fund) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, fund := range funds {
		if fund.ID == "" || fund.Name == "" {
			return errors.New("基金資料缺少 ID 或名稱")
		}
		_, err = tx.Exec(ctx, `INSERT INTO funds(id,name,manager,nisa_tsumitate,nisa_growth,paypay_url,history_url,refreshed_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,manager=excluded.manager,nisa_tsumitate=excluded.nisa_tsumitate,nisa_growth=excluded.nisa_growth,paypay_url=excluded.paypay_url,history_url=excluded.history_url,refreshed_at=excluded.refreshed_at`, fund.ID, fund.Name, fund.Manager, fund.NISATsumitate, fund.NISAGrowth, fund.PayPayURL, fund.HistoryURL, fund.RefreshedAt.UTC())
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) FindFunds(query string, limit int) ([]domain.Fund, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(context.Background(), `SELECT id,name,manager,nisa_tsumitate,nisa_growth,paypay_url,history_url,refreshed_at FROM funds WHERE name ILIKE $1 ORDER BY name LIMIT $2`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostgresFunds(rows)
}

func (s *PostgresStore) LastFundRefresh() (time.Time, error) {
	var refreshed *time.Time
	if err := s.pool.QueryRow(context.Background(), `SELECT MAX(refreshed_at) FROM funds`).Scan(&refreshed); err != nil {
		return time.Time{}, err
	}
	if refreshed == nil {
		return time.Time{}, nil
	}
	return refreshed.UTC(), nil
}

func (s *PostgresStore) Fund(id string) (domain.Fund, error) {
	row := s.pool.QueryRow(context.Background(), `SELECT id,name,manager,nisa_tsumitate,nisa_growth,paypay_url,history_url,refreshed_at FROM funds WHERE id=$1`, id)
	fund, err := scanPostgresFund(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Fund{}, fmt.Errorf("找不到基金：%s", id)
	}
	return fund, err
}

func (s *PostgresStore) ReplacePrices(fundID string, points []domain.PricePoint) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM prices WHERE fund_id=$1`, fundID); err != nil {
		return err
	}
	for _, point := range points {
		if point.NAV <= 0 || point.Date.IsZero() {
			return errors.New("歷史淨值資料無效")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO prices(fund_id,date,nav,source_url) VALUES($1,$2,$3,$4)`, fundID, point.Date.UTC(), point.NAV, point.SourceURL); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) Prices(fundID string) ([]domain.PricePoint, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT fund_id,date,nav,source_url FROM prices WHERE fund_id=$1 ORDER BY date`, fundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []domain.PricePoint{}
	for rows.Next() {
		var point domain.PricePoint
		if err := rows.Scan(&point.FundID, &point.Date, &point.NAV, &point.SourceURL); err != nil {
			return nil, err
		}
		point.Date = point.Date.UTC()
		points = append(points, point)
	}
	return points, rows.Err()
}

func (s *PostgresStore) ReplaceCPI(points []domain.CPIPoint) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM cpi`); err != nil {
		return err
	}
	for _, point := range points {
		if point.Index <= 0 || point.Date.IsZero() {
			return errors.New("CPI 資料格式無效")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO cpi(date,value,source_url) VALUES($1,$2,$3)`, point.Date.UTC(), point.Index, point.SourceURL); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CPI() ([]domain.CPIPoint, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT date,value,source_url FROM cpi ORDER BY date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []domain.CPIPoint{}
	for rows.Next() {
		var point domain.CPIPoint
		if err := rows.Scan(&point.Date, &point.Index, &point.SourceURL); err != nil {
			return nil, err
		}
		point.Date = point.Date.UTC()
		points = append(points, point)
	}
	return points, rows.Err()
}

func (s *PostgresStore) Insights(fundID string) ([]domain.Insight, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT id,fund_id,publisher,title,summary,source_url,published_at FROM insights WHERE fund_id=$1 ORDER BY published_at DESC LIMIT 10`, fundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostgresInsights(rows)
}

type postgresScanner interface{ Scan(...any) error }

func scanPostgresFund(row postgresScanner) (domain.Fund, error) {
	var fund domain.Fund
	err := row.Scan(&fund.ID, &fund.Name, &fund.Manager, &fund.NISATsumitate, &fund.NISAGrowth, &fund.PayPayURL, &fund.HistoryURL, &fund.RefreshedAt)
	fund.RefreshedAt = fund.RefreshedAt.UTC()
	return fund, err
}

func scanPostgresFunds(rows pgx.Rows) ([]domain.Fund, error) {
	result := []domain.Fund{}
	for rows.Next() {
		fund, err := scanPostgresFund(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, fund)
	}
	return result, rows.Err()
}

func scanPostgresInsights(rows pgx.Rows) ([]domain.Insight, error) {
	result := []domain.Insight{}
	for rows.Next() {
		var insight domain.Insight
		if err := rows.Scan(&insight.ID, &insight.FundID, &insight.Publisher, &insight.Title, &insight.Summary, &insight.SourceURL, &insight.PublishedAt); err != nil {
			return nil, err
		}
		insight.PublishedAt = insight.PublishedAt.UTC()
		result = append(result, insight)
	}
	return result, rows.Err()
}
