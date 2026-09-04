package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"paypay-nisa-analyzer/internal/app"
	"paypay-nisa-analyzer/internal/sources"
	"paypay-nisa-analyzer/internal/store"
)

func main() {
	dataDir := getenv("NISA_DATA_DIR", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dataDir, "nisa-analyzer.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// PublicSource only requests public, unauthenticated PayPay pages. Historical
	// providers are deliberately separate because each investment manager publishes
	// adjusted NAV data in a different format and under its own terms.
	server := app.New(db, sources.NewPayPayPublicSource(http.DefaultClient), log.Default())
	httpServer := &http.Server{Addr: ":" + getenv("PORT", "8080"), Handler: server.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("PayPay NISA 分析器已啟動：http://localhost%s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
