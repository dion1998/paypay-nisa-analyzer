package store

import (
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

// Repository 定義應用程式的資料存取邊界；正式環境使用 Supabase PostgreSQL
// 支援的 PostgresStore 實作。
type Repository interface {
	UpsertFunds([]domain.Fund) error
	FindFunds(string, int) ([]domain.Fund, error)
	LastFundRefresh() (time.Time, error)
	Fund(string) (domain.Fund, error)
	ReplacePrices(string, []domain.PricePoint) error
	Prices(string) ([]domain.PricePoint, error)
	ReplaceCPI([]domain.CPIPoint) error
	CPI() ([]domain.CPIPoint, error)
	Insights(string) ([]domain.Insight, error)
}
