package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"paypay-nisa-analyzer/internal/domain"
	"paypay-nisa-analyzer/internal/store"
)

type stubCatalog struct{}

func (stubCatalog) FetchFunds(context.Context) ([]domain.Fund, error) { return nil, nil }

func TestAnalysisAPI(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	app := New(db, stubCatalog{}, log.New(io.Discard, "", 0))
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
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := New(db, stubCatalog{}, log.New(io.Discard, "", 0))
	request := httptest.NewRequest(http.MethodPost, "/api/analysis", bytes.NewBufferString(`{"fundId":""}`))
	response := httptest.NewRecorder()
	app.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
}
