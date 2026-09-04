package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"paypay-nisa-analyzer/internal/sources"
	"paypay-nisa-analyzer/internal/store"
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL (or DATABASE_URL)")
	fundID := flag.String("fund", "", "fund ID returned by /api/funds")
	csvPath := flag.String("csv", "", "official adjusted NAV CSV path")
	sourceURL := flag.String("source", "", "official source URL for this CSV")
	flag.Parse()
	if *databaseURL == "" || *fundID == "" || *csvPath == "" || *sourceURL == "" {
		log.Fatal("必須提供 DATABASE_URL（或 -database-url）、-fund、-csv、-source")
	}
	file, err := os.Open(*csvPath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	points, err := sources.ParseAdjustedNAVCSV(file, *fundID, *sourceURL)
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.OpenPostgres(context.Background(), *databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Fund(*fundID); err != nil {
		log.Fatal(err)
	}
	if err := db.ReplacePrices(*fundID, points); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("已匯入 %d 筆調整後淨值資料。\n", len(points))
}
