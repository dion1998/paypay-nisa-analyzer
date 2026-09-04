package domain

import (
	"math"
	"testing"
	"time"
)

func TestCalculateUsesEndOfMonthContributions(t *testing.T) {
	points := makePoints(73, func(_ int) float64 { return 1.01 })
	result, err := Calculate(Fund{ID: "fund", Name: "測試基金", NISATsumitate: true}, points, 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := 1000.0
	for month := 1; month <= 60; month++ {
		want = want*1.01 + 100
	}
	if math.Abs(result.P50-want) > .01 {
		t.Fatalf("P50 = %.2f, want %.2f", result.P50, want)
	}
	if result.SampleCount != 13 {
		t.Fatalf("sample count = %d, want 13", result.SampleCount)
	}
}

func TestCalculateRejectsInsufficientHistory(t *testing.T) {
	_, err := Calculate(Fund{}, makePoints(60, func(_ int) float64 { return 1 }), 0, 100)
	if err != ErrInsufficientHistory {
		t.Fatalf("error = %v", err)
	}
}

func TestHoldingRecommendationFindsOneYear(t *testing.T) {
	points := makePoints(61, func(_ int) float64 { return 1.02 })
	result, err := Calculate(Fund{}, points, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.RecommendedYears != 1 {
		t.Fatalf("years = %d, want 1", result.RecommendedYears)
	}
	if result.PositiveReturnRate < .80 {
		t.Fatalf("rate = %v", result.PositiveReturnRate)
	}
}

func makePoints(count int, growth func(int) float64) []PricePoint {
	points := make([]PricePoint, count)
	nav := 100.0
	start := time.Date(2018, 1, 28, 0, 0, 0, 0, time.UTC)
	for i := range points {
		if i > 0 {
			nav *= growth(i)
		}
		points[i] = PricePoint{FundID: "fund", Date: start.AddDate(0, i, 0), NAV: nav, SourceURL: "https://issuer.example/history"}
	}
	return points
}
