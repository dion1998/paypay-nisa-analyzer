package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"paypay-nisa-analyzer/internal/app"
	"paypay-nisa-analyzer/internal/sources"
	"paypay-nisa-analyzer/internal/store"
)

func main() {
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	db, err := store.OpenPostgres(startupCtx, os.Getenv("DATABASE_URL"))
	cancelStartup()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 僅讀取 PayPay 不需登入的公開頁面。各投信公司的還原淨值格式與使用條款
	// 不同，因此歷史資料來源刻意分開處理。
	server := app.New(db, sources.NewPayPayPublicSource(http.DefaultClient), log.Default())
	httpServer := &http.Server{Addr: ":" + getenv("PORT", "8080"), Handler: server.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("PayPay NISA API 已啟動：http://localhost%s", httpServer.Addr)
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
