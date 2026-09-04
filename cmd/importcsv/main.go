package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"paypay-nisa-analyzer/internal/sources"
	"paypay-nisa-analyzer/internal/store"
)

func main() {
	database := flag.String("db", "data/nisa-analyzer.db", "SQLite database path")
	fundID := flag.String("fund", "", "fund ID returned by /api/funds")
	csvPath := flag.String("csv", "", "official adjusted NAV CSV path")
	sourceURL := flag.String("source", "", "official source URL for this CSV")
	flag.Parse()
	if *fundID == "" || *csvPath == "" || *sourceURL == "" {
		log.Fatal("必須提供 -fund、-csv、-source")
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
	db, err := store.Open(*database)
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
