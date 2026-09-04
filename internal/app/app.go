package app

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
	"paypay-nisa-analyzer/internal/domain"
	"paypay-nisa-analyzer/internal/sources"
	"paypay-nisa-analyzer/internal/store"
)

//go:embed templates/index.html static/app.css static/app.js
var assets embed.FS

type App struct {
	store   *store.Store
	catalog sources.FundCatalogSource
	logger  *log.Logger
	tpl     *template.Template
}

func New(data *store.Store, catalog sources.FundCatalogSource, logger *log.Logger) *App {
	a := newApp(data, catalog, logger)
	go a.refreshIfStale()
	return a
}

func newApp(data *store.Store, catalog sources.FundCatalogSource, logger *log.Logger) *App {
	tpl := template.Must(template.ParseFS(assets, "templates/index.html"))
	return &App{store: data, catalog: catalog, logger: logger, tpl: tpl}
}

func (a *App) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", a.index)
	static, _ := fs.Sub(assets, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	r.Get("/api/funds", a.findFunds)
	r.Post("/api/analysis", a.analyze)
	r.Post("/api/data/refresh", a.refresh)
	r.Get("/api/insights/{fundID}", a.insights)
	return r
}

func (a *App) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = a.tpl.Execute(w, map[string]any{"Title": "PayPay NISA 投信分析器"})
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
		writeError(w, http.StatusBadRequest, errors.New("請選擇基金"))
		return
	}
	fund, err := a.store.Fund(request.FundID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	prices, err := a.store.Prices(fund.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	analysis, err := domain.Calculate(fund, prices, request.InitialAmount, request.MonthlyAmount)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, analysis)
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
		// The UI treats old observations as context only, never as forecast input.
		if time.Since(insights[i].PublishedAt) > 90*24*time.Hour {
			insights[i].Summary = "【可能過期】" + insights[i].Summary
		}
	}
	writeJSON(w, http.StatusOK, insights)
}

func (a *App) refresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	count, err := a.refreshCatalog(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fundsUpdated": count, "message": "已更新公開基金清單。歷史資料與觀點資料僅在已設定且允許的發行商來源中更新。"})
}

func (a *App) refreshIfStale() {
	last, err := a.store.LastFundRefresh()
	if err != nil {
		a.logger.Printf("讀取基金同步時間失敗：%v", err)
		return
	}
	if !last.IsZero() && time.Since(last) < 24*time.Hour {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := a.refreshCatalog(ctx); err != nil {
		a.logger.Printf("公開基金清單未更新：%v", err)
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

func decodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(target)
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
