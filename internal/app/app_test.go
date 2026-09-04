package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"paypay-nisa-analyzer/internal/domain"
	"paypay-nisa-analyzer/internal/store"

	_ "modernc.org/sqlite"
)

type stubCatalog struct {
	funds []domain.Fund
	err   error
}

func (s stubCatalog) FetchFunds(context.Context) ([]domain.Fund, error) { return s.funds, s.err }

func newTestApp(t *testing.T, catalog stubCatalog) (*App, *store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db, catalog, log.New(io.Discard, "", 0)), db, path
}

func TestAnalysisAPI(t *testing.T) {
	app, db, _ := newTestApp(t, stubCatalog{})
	fund := domain.Fund{ID: "fund-1", Name: "測試投信", NISATsumitate: true, RefreshedAt: time.Now().UTC()}
	if err := db.UpsertFunds([]domain.Fund{fund}); err != nil {
		t.Fatal(err)
	}
	prices := make([]domain.PricePoint, 73)
	nav := 100.0
	start := time.Date(2019, 1, 28, 0, 0, 0, 0, time.UTC)
	for i := range prices {
		if i > 0 {
			nav *= 1.01
		}
		prices[i] = domain.PricePoint{FundID: fund.ID, Date: start.AddDate(0, i, 0), NAV: nav, SourceURL: "https://issuer.example"}
	}
	if err := db.ReplacePrices(fund.ID, prices); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"fundId":"fund-1","initialAmount":100000,"monthlyAmount":30000}`)
	request := httptest.NewRequest(http.MethodPost, "/api/analysis", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var result domain.Analysis
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.P50 <= result.TotalContributions || result.SampleCount == 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAnalysisAPIRejectsInvalidInput(t *testing.T) {
	app, _, _ := newTestApp(t, stubCatalog{})
	request := httptest.NewRequest(http.MethodPost, "/api/analysis", bytes.NewBufferString(`{"fundId":""}`))
	response := httptest.NewRecorder()
	app.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRoutesServePageAndFundSearch(t *testing.T) {
	app, db, _ := newTestApp(t, stubCatalog{})
	fund := domain.Fund{ID: "fund", Name: "Alpha Fund", RefreshedAt: time.Now().UTC()}
	if err := db.UpsertFunds([]domain.Fund{fund}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/static/app.css", http.StatusOK},
		{"/api/funds?q=Alpha", http.StatusOK},
	} {
		response := httptest.NewRecorder()
		app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target.path, nil))
		if response.Code != target.want {
			t.Fatalf("%s status = %d: %s", target.path, response.Code, response.Body.String())
		}
	}
}

func TestAnalysisAPIErrors(t *testing.T) {
	app, db, path := newTestApp(t, stubCatalog{})
	cases := []struct {
		name string
		body string
		want int
	}{
		{"malformed", "{", http.StatusBadRequest},
		{"blank id", `{"fundId":" "}`, http.StatusBadRequest},
		{"missing fund", `{"fundId":"missing","initialAmount":1}`, http.StatusNotFound},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/analysis", bytes.NewBufferString(tt.body)))
			if response.Code != tt.want {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
		})
	}
	if err := db.UpsertFunds([]domain.Fund{{ID: "short", Name: "Short", RefreshedAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/analysis", bytes.NewBufferString(`{"fundId":"short"}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if _, err := db.Fund("short"); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplacePrices("short", []domain.PricePoint{{FundID: "short", Date: time.Now(), NAV: 100}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Fund("short"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Prices("short"); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplacePrices("short", nil); err != nil {
		t.Fatal(err)
	}
	// Removing the price table makes the handler exercise its database error response.
	if _, err := execRaw(path, `DROP TABLE prices`); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/analysis", bytes.NewBufferString(`{"fundId":"short"}`)))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("price failure status = %d", response.Code)
	}
}

func TestInsightsAndRefreshRoutes(t *testing.T) {
	fund := domain.Fund{ID: "fund", Name: "Fund", RefreshedAt: time.Now().UTC()}
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.UpsertFunds([]domain.Fund{fund}); err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	app := New(db, stubCatalog{funds: []domain.Fund{fund}}, log.New(io.Discard, "", 0))
	_, err = seed.Exec(`INSERT INTO insights(fund_id,publisher,title,summary,source_url,published_at) VALUES(?,?,?,?,?,?)`, fund.ID, "Issuer", "Old", "context", "https://example.test", time.Now().Add(-91*24*time.Hour).UTC().Format(time.RFC3339))
	_ = seed.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		path string
		want int
	}{
		{"/api/insights/fund", http.StatusOK},
		{"/api/insights/missing", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target.path, nil))
		if response.Code != target.want {
			t.Fatalf("%s status = %d", target.path, response.Code)
		}
	}
	if _, err := execRaw(path, `DROP TABLE insights`); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/insights/fund", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("insights failure status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	app.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/data/refresh", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("refresh status = %d: %s", response.Code, response.Body.String())
	}
}

// dbInternalExec opens a second connection because Store deliberately keeps its
// SQLite handle private; it is used only to create otherwise unreachable read errors.
func execRaw(path, statement string) (sql.Result, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.Exec(statement)
}

func TestRefreshErrorPathsAndHelpers(t *testing.T) {
	app, db, _ := newTestApp(t, stubCatalog{err: errors.New("upstream")})
	response := httptest.NewRecorder()
	app.refresh(response, httptest.NewRequest(http.MethodPost, "/api/data/refresh", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", response.Code)
	}
	if _, err := app.refreshCatalog(context.Background()); err == nil {
		t.Fatal("expected catalogue error")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	app.refreshIfStale()
	response = httptest.NewRecorder()
	app.findFunds(response, httptest.NewRequest(http.MethodGet, "/api/funds", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	var target map[string]any
	if err := decodeJSON(httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"ok":true}`)), &target); err != nil || target["ok"] != true {
		t.Fatalf("decode = %#v, %v", target, err)
	}
}
