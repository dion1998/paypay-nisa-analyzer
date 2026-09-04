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

func TestCalculatorHelpersAndNISANotes(t *testing.T) {
	if got := monthlyPoints(nil); len(got) != 0 {
		t.Fatalf("monthly points = %#v", got)
	}
	points := []PricePoint{
		{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), NAV: 1},
		{Date: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), NAV: 2},
		{Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), NAV: 3},
		{Date: time.Time{}, NAV: 4},
		{Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), NAV: 0},
	}
	monthly := monthlyPoints(points)
	if len(monthly) != 2 || monthly[0].NAV != 2 || monthly[1].NAV != 3 {
		t.Fatalf("monthly = %#v", monthly)
	}
	if percentile(nil, .5) != 0 || percentile([]float64{1, 2}, -1) != 1 || percentile([]float64{1, 2}, 2) != 2 || percentile([]float64{1, 2, 3, 4}, .5) != 2.5 {
		t.Fatal("unexpected percentile")
	}
	if maxDrawdown([]PricePoint{{NAV: 1}, {NAV: 2}}) != 0 || math.Abs(maxDrawdown([]PricePoint{{NAV: 10}, {NAV: 20}, {NAV: 5}})+.75) > 1e-9 {
		t.Fatal("unexpected drawdown")
	}
	if nisaNote(Fund{}, 100, 10) == "" || nisaNote(Fund{NISATsumitate: true}, 100, 10) == "" || nisaNote(Fund{NISAGrowth: true}, 100, 10) == "" || nisaNote(Fund{NISATsumitate: true, NISAGrowth: true}, 100, 10) == "" {
		t.Fatal("expected NISA notes")
	}
	if formatYen(1234.4) == "" || strconvFormat(0) != "0" || strconvFormat(1234567) != "1,234,567" || strconvFormat(-12345) != "-12,345" {
		t.Fatal("unexpected yen formatting")
	}
}

func TestCalculateValidationAndRecommendationFallback(t *testing.T) {
	if _, err := Calculate(Fund{}, makePoints(61, func(int) float64 { return 1 }), -1, 0); err == nil {
		t.Fatal("expected invalid amount")
	}
	volatile := makePoints(61, func(int) float64 { return .99 })
	result, err := Calculate(Fund{NISAGrowth: true}, volatile, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.RecommendedYears != 0 || result.PositiveReturnRate < 0 || result.NISANote == "" || result.Methodology == "" || result.Disclaimer == "" {
		t.Fatalf("unexpected fallback result %#v", result)
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
