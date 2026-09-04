package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "analyzer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func testFund(id string) domain.Fund {
	return domain.Fund{ID: id, Name: "Test fund " + id, Manager: "Manager", NISATsumitate: true, NISAGrowth: true, PayPayURL: "https://example.test/fund", HistoryURL: "https://example.test/nav", RefreshedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func TestStoreFundLifecycle(t *testing.T) {
	s := newTestStore(t)
	if got, err := s.LastFundRefresh(); err != nil || !got.IsZero() {
		t.Fatalf("empty refresh = %v, %v", got, err)
	}
	first, second := testFund("fund-a"), testFund("fund-b")
	if err := s.UpsertFunds([]domain.Fund{first, second}); err != nil {
		t.Fatal(err)
	}
	first.Name = "Updated fund"
	if err := s.UpsertFunds([]domain.Fund{first}); err != nil {
		t.Fatal(err)
	}
	funds, err := s.FindFunds("fund", 0)
	if err != nil || len(funds) != 2 || funds[0].Name != "Test fund fund-b" || !funds[1].NISAGrowth {
		t.Fatalf("find = %#v, %v", funds, err)
	}
	if funds, err = s.FindFunds("Updated", 99); err != nil || len(funds) != 1 || funds[0].ID != first.ID {
		t.Fatalf("limited find = %#v, %v", funds, err)
	}
	got, err := s.Fund(first.ID)
	if err != nil || got.Name != first.Name || !got.NISATsumitate || got.RefreshedAt != first.RefreshedAt {
		t.Fatalf("fund = %#v, %v", got, err)
	}
	if _, err := s.Fund("missing"); err == nil {
		t.Fatal("expected missing fund error")
	}
	if got, err := s.LastFundRefresh(); err != nil || !got.Equal(first.RefreshedAt) {
		t.Fatalf("refresh = %v, %v", got, err)
	}
}

func TestStorePricesAndInsights(t *testing.T) {
	s := newTestStore(t)
	fund := testFund("fund-a")
	if err := s.UpsertFunds([]domain.Fund{fund}); err != nil {
		t.Fatal(err)
	}
	points := []domain.PricePoint{
		{FundID: fund.ID, Date: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), NAV: 100, SourceURL: "https://example.test/nav"},
		{FundID: fund.ID, Date: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), NAV: 110, SourceURL: "https://example.test/nav"},
	}
	if err := s.ReplacePrices(fund.ID, points); err != nil {
		t.Fatal(err)
	}
	got, err := s.Prices(fund.ID)
	if err != nil || len(got) != 2 || got[1].NAV != 110 || got[0].FundID != fund.ID {
		t.Fatalf("prices = %#v, %v", got, err)
	}
	_, err = s.db.Exec(`INSERT INTO insights(fund_id,publisher,title,summary,source_url,published_at) VALUES(?,?,?,?,?,?)`, fund.ID, "Issuer", "Older", "summary", "https://example.test/report", "2025-01-02T03:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`INSERT INTO insights(fund_id,publisher,title,summary,source_url,published_at) VALUES(?,?,?,?,?,?)`, fund.ID, "Issuer", "Newer", "summary", "https://example.test/report", "2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	insights, err := s.Insights(fund.ID)
	if err != nil || len(insights) != 2 || insights[0].Title != "Newer" {
		t.Fatalf("insights = %#v, %v", insights, err)
	}
}

func TestStoreValidationAndDatabaseErrors(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertFunds([]domain.Fund{{Name: "missing id"}}); err == nil {
		t.Fatal("expected fund validation error")
	}
	if err := s.ReplacePrices("missing", []domain.PricePoint{{NAV: 0}}); err == nil {
		t.Fatal("expected price validation error")
	}
	if err := s.ReplacePrices("missing", []domain.PricePoint{{Date: time.Now(), NAV: 1}}); err == nil {
		t.Fatal("expected foreign key error")
	}
	if _, err := s.db.Exec(`UPDATE funds SET refreshed_at='bad'`); err != nil {
		t.Fatal(err)
	}
	// An empty table has no malformed timestamp, so add a row directly for parser coverage.
	if _, err := s.db.Exec(`INSERT INTO funds(id,name,refreshed_at) VALUES('bad-time','Bad','not-a-time')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LastFundRefresh(); err == nil {
		t.Fatal("expected invalid refresh timestamp")
	}
	if _, err := s.Fund("bad-time"); err == nil {
		t.Fatal("expected invalid fund timestamp")
	}
	if _, err := s.FindFunds("Bad", 1); err == nil {
		t.Fatal("expected invalid fund scan")
	}
	if _, err := s.db.Exec(`INSERT INTO prices(fund_id,date,nav,source_url) VALUES('bad-time','not-a-date',1,'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prices("bad-time"); err == nil {
		t.Fatal("expected invalid price date")
	}
	if _, err := s.db.Exec(`INSERT INTO insights(fund_id,publisher,title,summary,source_url,published_at) VALUES('bad-time','p','t','s','u','not-a-time')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Insights("bad-time"); err == nil {
		t.Fatal("expected invalid insight date")
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindFunds("", 1); err == nil {
		t.Fatal("expected closed database error")
	}
	if err := s.migrate(); err == nil {
		t.Fatal("expected migration error")
	}
	if err := s.UpsertFunds(nil); err == nil {
		t.Fatal("expected transaction error")
	}
	if err := s.ReplacePrices("x", nil); err == nil {
		t.Fatal("expected transaction error")
	}
	if _, err := s.LastFundRefresh(); err == nil {
		t.Fatal("expected refresh error")
	}
	if _, err := s.Fund("x"); err == nil {
		t.Fatal("expected fund query error")
	}
	if _, err := s.Prices("x"); err == nil {
		t.Fatal("expected prices query error")
	}
	if _, err := s.Insights("x"); err == nil {
		t.Fatal("expected insights query error")
	}
}

type scanError struct{ err error }

func (s scanError) Scan(...any) error { return s.err }

func TestStoreHelpers(t *testing.T) {
	if _, err := scanFund(scanError{errors.New("scan")}); err == nil {
		t.Fatal("expected scan error")
	}
	if boolInt(true) != 1 || boolInt(false) != 0 {
		t.Fatal("unexpected bool conversion")
	}
	originalOpen := openSQLite
	openSQLite = func(string, string) (*sql.DB, error) { return nil, errors.New("open failed") }
	if _, err := Open("ignored"); err == nil {
		t.Fatal("expected open error")
	}
	openSQLite = originalOpen
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(parent, "db.sqlite")); err == nil {
		t.Fatal("expected open/migrate error")
	}
	_, _ = sql.Open("sqlite", ":memory:")
}

func TestStorePrepareAndScanErrors(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_fund_insert BEFORE INSERT ON funds BEGIN SELECT RAISE(FAIL, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFunds([]domain.Fund{testFund("fund")}); err == nil {
		t.Fatal("expected insert error")
	}

	s = newTestStore(t)
	if _, err := s.db.Exec(`DROP TABLE prices`); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePrices("fund", nil); err == nil {
		t.Fatal("expected delete error")
	}

	s = newTestStore(t)
	if _, err := s.db.Exec(`DROP TABLE prices; CREATE TABLE prices (fund_id TEXT, date TEXT, nav TEXT, source_url TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO prices VALUES('fund','2026-01-01','not-a-number','url')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prices("fund"); err == nil {
		t.Fatal("expected price scan error")
	}

	s = newTestStore(t)
	if _, err := s.db.Exec(`DROP TABLE insights; CREATE TABLE insights (id TEXT, fund_id TEXT, publisher TEXT, title TEXT, summary TEXT, source_url TEXT, published_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO insights VALUES('not-an-id','fund','publisher','title','summary','url','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Insights("fund"); err == nil {
		t.Fatal("expected insight scan error")
	}
}
