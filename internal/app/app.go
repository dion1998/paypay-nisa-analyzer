package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"paypay-nisa-analyzer/internal/domain"
	"paypay-nisa-analyzer/internal/sources"
	"paypay-nisa-analyzer/internal/store"
)

type App struct {
	store   store.Repository
	catalog sources.FundCatalogSource
	cpi     sources.CPISource
	logger  *log.Logger
}

func New(data store.Repository, catalog sources.FundCatalogSource, logger *log.Logger) *App {
	a := newApp(data, catalog, logger)
	// 啟動時只在公開基金清單過期時背景更新，不阻塞 API 開始接收請求。
	go a.refreshIfStale()
	return a
}
func newApp(data store.Repository, catalog sources.FundCatalogSource, logger *log.Logger) *App {
	return &App{store: data, catalog: catalog, cpi: sources.StatisticsBureauCPISource{Client: http.DefaultClient}, logger: logger}
}

func (a *App) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(cors)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/api/funds", a.findFunds)
	r.Post("/api/analysis", a.analyze)
	r.Post("/api/data/refresh", a.refresh)
	r.Get("/api/history/{fundID}", a.history)
	r.Get("/api/insights/{fundID}", a.insights)
	return r
}
func (a *App) findFunds(w http.ResponseWriter, r *http.Request) {
	funds, err := a.store.FindFunds(strings.TrimSpace(r.URL.Query().Get("q")), 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, funds)
}
func (a *App) analyze(w http.ResponseWriter, r *http.Request) {
	var request domain.AnalysisRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("請提供有效的 JSON 輸入"))
		return
	}
	if strings.TrimSpace(request.FundID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("請選擇投資信託"))
		return
	}
	fund, err := a.store.Fund(request.FundID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	prices, err := a.ensureHistory(r.Context(), fund)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cpi, cpiErr := a.ensureCPI(r.Context())
	if cpiErr != nil {
		a.logger.Printf("CPI unavailable: %v", cpiErr)
	}
	analysis, err := domain.Calculate(fund, prices, cpi, request.InitialAmount, request.MonthlyAmount)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, analysis)
}
func (a *App) history(w http.ResponseWriter, r *http.Request) {
	fundID := chi.URLParam(r, "fundID")
	fund, err := a.store.Fund(fundID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	points, err := a.ensureHistory(r.Context(), fund)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fund": fund, "points": points, "dataAsOf": lastPriceDate(points)})
}
func (a *App) insights(w http.ResponseWriter, r *http.Request) {
	fundID := chi.URLParam(r, "fundID")
	if _, err := a.store.Fund(fundID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	insights, err := a.store.Insights(fundID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for i := range insights {
		if time.Since(insights[i].PublishedAt) > 90*24*time.Hour {
			insights[i].Summary = "觀點可能過期：" + insights[i].Summary
		}
	}
	writeJSON(w, http.StatusOK, insights)
}
func (a *App) refresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	count, err := a.refreshCatalog(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	_, cpiErr := a.ensureCPI(ctx)
	message := "已更新公開基金清單。"
	if cpiErr != nil {
		message += " CPI 暫時無法更新，持有年限會等待下次資料同步。"
	}
	writeJSON(w, http.StatusOK, map[string]any{"fundsUpdated": count, "message": message})
}
func (a *App) refreshIfStale() {
	last, err := a.store.LastFundRefresh()
	if err != nil {
		a.logger.Printf("read fund refresh time: %v", err)
		return
	}
	if !last.IsZero() && time.Since(last) < 24*time.Hour {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if _, err := a.refreshCatalog(ctx); err != nil {
		a.logger.Printf("refresh fund catalog: %v", err)
	}
	if _, err := a.ensureCPI(ctx); err != nil {
		a.logger.Printf("refresh CPI: %v", err)
	}
}
func (a *App) refreshCatalog(ctx context.Context) (int, error) {
	funds, err := a.catalog.FetchFunds(ctx)
	if err != nil {
		return 0, err
	}
	if err := a.store.UpsertFunds(funds); err != nil {
		return 0, err
	}
	return len(funds), nil
}

func (a *App) ensureHistory(ctx context.Context, fund domain.Fund) ([]domain.PricePoint, error) {
	points, err := a.store.Prices(fund.ID)
	if err != nil {
		return nil, err
	}
	// 至少有 13 個月資料才足以計算 12 個月報酬，無須再向發行商抓取。
	if len(points) >= 13 {
		return points, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	if fetched, supported, fetchErr := sources.FetchEMAXISReinvestedNAV(ctx, client, fund); supported {
		if fetchErr != nil {
			return points, fetchErr
		}
		if err := a.store.ReplacePrices(fund.ID, fetched); err != nil {
			return points, err
		}
		return fetched, nil
	}
	if fetched, supported, fetchErr := sources.FetchNissayReinvestedNAV(ctx, client, fund); supported {
		if fetchErr != nil {
			return points, fetchErr
		}
		if err := a.store.ReplacePrices(fund.ID, fetched); err != nil {
			return points, err
		}
		return fetched, nil
	}
	return points, nil
}
func (a *App) ensureCPI(ctx context.Context) ([]domain.CPIPoint, error) {
	points, err := a.store.CPI()
	if err == nil && len(points) >= 24 && time.Since(points[len(points)-1].Date) < 45*24*time.Hour {
		return points, nil
	}
	if a.cpi == nil {
		return points, errors.New("CPI source is not configured")
	}
	fresh, fetchErr := a.cpi.FetchCPI(ctx)
	if fetchErr != nil {
		return points, fetchErr
	}
	if err := a.store.ReplaceCPI(fresh); err != nil {
		return points, err
	}
	return fresh, nil
}
func lastPriceDate(points []domain.PricePoint) time.Time {
	if len(points) == 0 {
		return time.Time{}
	}
	return points[len(points)-1].Date
}
func decodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(target)
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := os.Getenv("CORS_ORIGIN")
		if allowed == "" {
			allowed = "http://localhost:5173"
		}
		if origin == allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
