package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"paypay-nisa-analyzer/internal/domain"
	"paypay-nisa-analyzer/internal/store"
)

type stubCatalog struct {
	funds []domain.Fund
	err   error
}

func (s stubCatalog) FetchFunds(context.Context) ([]domain.Fund, error) { return s.funds, s.err }

type memoryStore struct {
	funds    map[string]domain.Fund
	prices   map[string][]domain.PricePoint
	cpi      []domain.CPIPoint
	insights map[string][]domain.Insight
}

var _ store.Repository = (*memoryStore)(nil)

func newMemoryStore() *memoryStore {
	now := time.Now().UTC()
	cpi := make([]domain.CPIPoint, 24)
	for i := range cpi {
		cpi[i] = domain.CPIPoint{Date: now.AddDate(0, i-23, 0), Index: 100 + float64(i), SourceURL: "https://example.test/cpi"}
	}
	return &memoryStore{funds: map[string]domain.Fund{}, prices: map[string][]domain.PricePoint{}, cpi: cpi, insights: map[string][]domain.Insight{}}
}

func (s *memoryStore) UpsertFunds(funds []domain.Fund) error {
	for _, fund := range funds {
		if fund.ID == "" || fund.Name == "" {
			return errors.New("基金資料缺少 ID 或名稱")
		}
		s.funds[fund.ID] = fund
	}
	return nil
}
func (s *memoryStore) FindFunds(query string, limit int) ([]domain.Fund, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query = strings.ToLower(query)
	result := make([]domain.Fund, 0)
	for _, fund := range s.funds {
		if strings.Contains(strings.ToLower(fund.Name), query) {
			result = append(result, fund)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (s *memoryStore) LastFundRefresh() (time.Time, error) {
	var newest time.Time
	for _, fund := range s.funds {
		if fund.RefreshedAt.After(newest) {
			newest = fund.RefreshedAt
		}
	}
	return newest, nil
}
func (s *memoryStore) Fund(id string) (domain.Fund, error) {
	fund, ok := s.funds[id]
	if !ok {
		return domain.Fund{}, fmt.Errorf("找不到基金：%s", id)
	}
	return fund, nil
}
func (s *memoryStore) ReplacePrices(fundID string, points []domain.PricePoint) error {
	s.prices[fundID] = append([]domain.PricePoint(nil), points...)
	return nil
}
func (s *memoryStore) Prices(fundID string) ([]domain.PricePoint, error) {
	return append([]domain.PricePoint(nil), s.prices[fundID]...), nil
}
func (s *memoryStore) ReplaceCPI(points []domain.CPIPoint) error {
	s.cpi = append([]domain.CPIPoint(nil), points...)
	return nil
}
func (s *memoryStore) CPI() ([]domain.CPIPoint, error) {
	return append([]domain.CPIPoint(nil), s.cpi...), nil
}
func (s *memoryStore) Insights(fundID string) ([]domain.Insight, error) {
	return append([]domain.Insight(nil), s.insights[fundID]...), nil
}

func newTestApp(catalog stubCatalog) (*App, *memoryStore) {
	data := newMemoryStore()
	return newApp(data, catalog, log.New(io.Discard, "", 0)), data
}

func TestAnalysisAPI(t *testing.T) {
	app, data := newTestApp(stubCatalog{})
	fund := domain.Fund{ID: "fund-1", Name: "測試投信", NISATsumitate: true, RefreshedAt: time.Now().UTC()}
	if err := data.UpsertFunds([]domain.Fund{fund}); err != nil {
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
	if err := data.ReplacePrices(fund.ID, prices); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/analysis", bytes.NewBufferString(`{"fundId":"fund-1","initialAmount":100000,"monthlyAmount":30000}`))
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

func TestRoutesAndValidation(t *testing.T) {
	app, data := newTestApp(stubCatalog{})
	fund := domain.Fund{ID: "fund", Name: "Alpha Fund", RefreshedAt: time.Now().UTC()}
	if err := data.UpsertFunds([]domain.Fund{fund}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/healthz", "", http.StatusOK},
		{http.MethodGet, "/api/funds?q=Alpha", "", http.StatusOK},
		{http.MethodPost, "/api/analysis", `{"fundId":""}`, http.StatusBadRequest},
		{http.MethodPost, "/api/analysis", `{"fundId":"missing"}`, http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		app.Routes().ServeHTTP(response, httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body)))
		if response.Code != test.want {
			t.Fatalf("%s %s status = %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestCORSOnlyAllowsConfiguredFrontend(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "https://app.example.test")
	app, _ := newTestApp(stubCatalog{})
	for _, test := range []struct{ origin, want string }{{"https://app.example.test", "https://app.example.test"}, {"https://attacker.example.test", ""}} {
		request := httptest.NewRequest(http.MethodOptions, "/api/funds", nil)
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		app.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != test.want {
			t.Fatalf("origin %q: status=%d allow=%q", test.origin, response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}

func TestInsightsAndRefreshRoutes(t *testing.T) {
	fund := domain.Fund{ID: "fund", Name: "Fund", RefreshedAt: time.Now().UTC()}
	app, data := newTestApp(stubCatalog{funds: []domain.Fund{fund}})
	if err := data.UpsertFunds([]domain.Fund{fund}); err != nil {
		t.Fatal(err)
	}
	data.insights[fund.ID] = []domain.Insight{{FundID: fund.ID, Title: "Old", PublishedAt: time.Now().Add(-91 * 24 * time.Hour)}}
	for _, test := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/insights/fund", http.StatusOK},
		{http.MethodGet, "/api/insights/missing", http.StatusNotFound},
		{http.MethodPost, "/api/data/refresh", http.StatusOK},
	} {
		response := httptest.NewRecorder()
		app.Routes().ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.want {
			t.Fatalf("%s %s status = %d", test.method, test.path, response.Code)
		}
	}
}

func TestRefreshErrorAndFreshStartup(t *testing.T) {
	app, _ := newTestApp(stubCatalog{err: errors.New("upstream")})
	response := httptest.NewRecorder()
	app.refresh(response, httptest.NewRequest(http.MethodPost, "/api/data/refresh", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", response.Code)
	}
	data := newMemoryStore()
	if err := data.UpsertFunds([]domain.Fund{{ID: "fresh", Name: "Fresh", RefreshedAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if got := New(data, stubCatalog{}, log.New(io.Discard, "", 0)); got == nil {
		t.Fatal("expected app")
	}
}
