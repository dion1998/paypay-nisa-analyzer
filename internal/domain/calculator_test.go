package domain

import (
	"math"
	"testing"
	"time"
)

func TestCalculateUsesEndOfMonthContributions(t *testing.T) {
	points := makePoints(73, func(int) float64 { return 1.01 })
	result, err := Calculate(Fund{ID: "fund", Name: "Test Fund", NISATsumitate: true}, points, makeCPI(73), 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := 1000.0
	for month := 1; month <= 60; month++ {
		want = want*1.01 + 100
	}
	if math.Abs(result.P50-want) > .01 {
		t.Fatalf("P50 %.2f, want %.2f", result.P50, want)
	}
	if result.SampleCount != 13 {
		t.Fatalf("samples %d", result.SampleCount)
	}
}
func TestCalculateSupportsShortHistoryWithBootstrap(t *testing.T) {
	result, err := Calculate(Fund{ID: "short"}, makePoints(24, func(int) float64 { return 1.005 }), makeCPI(24), 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.SampleCount != bootstrapPaths || result.P50 <= 0 {
		t.Fatalf("unexpected short history %#v", result)
	}
}
func TestCalculateRejectsLessThanTwelveMonths(t *testing.T) {
	_, err := Calculate(Fund{}, makePoints(12, func(int) float64 { return 1 }), nil, 0, 100)
	if err != ErrInsufficientHistory {
		t.Fatalf("error %v", err)
	}
}
func TestHoldingRecommendationUsesMultipleCriteria(t *testing.T) {
	result, err := Calculate(Fund{ID: "fund"}, makePoints(80, func(int) float64 { return 1.02 }), makeCPI(80), 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.RecommendedYears == 0 || !result.HoldingCriteria.Passed || result.HoldingCriteria.RealSuccessRate < .8 {
		t.Fatalf("unexpected criteria %#v", result.HoldingCriteria)
	}
}
func TestHoldingWithoutCPIIsExplicit(t *testing.T) {
	result, err := Calculate(Fund{ID: "fund"}, makePoints(61, func(int) float64 { return 1.01 }), nil, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.RecommendedYears != 0 || result.HoldingCriteria.CPIAvailable || len(result.HoldingCriteria.FailedReasons) == 0 {
		t.Fatalf("unexpected CPI fallback %#v", result.HoldingCriteria)
	}
}
func TestHelpers(t *testing.T) {
	if len(monthlyPoints(nil)) != 0 || percentile(nil, .5) != 0 || percentile([]float64{1, 2, 3, 4}, .5) != 2.5 {
		t.Fatal("percentile/monthly helper")
	}
	if math.Abs(maxDrawdown([]PricePoint{{NAV: 10}, {NAV: 20}, {NAV: 5}})+.75) > 1e-9 {
		t.Fatal("drawdown")
	}
	if nisaNote(Fund{}, 100, 10) == "" || formatYen(1234.4) != "1,234" || strconvFormat(-12345) != "-12,345" {
		t.Fatal("format")
	}
	if wilsonLowerBound(8, 10) <= 0 || expectedShortfall([]float64{-2, -1, 1}, .1) != -2 {
		t.Fatal("risk helper")
	}
}
func TestInvalidAmount(t *testing.T) {
	if _, err := Calculate(Fund{}, makePoints(13, func(int) float64 { return 1 }), nil, -1, 0); err == nil {
		t.Fatal("want validation error")
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
func makeCPI(count int) []CPIPoint {
	points := make([]CPIPoint, count)
	start := time.Date(2018, 1, 28, 0, 0, 0, 0, time.UTC)
	for i := range points {
		points[i] = CPIPoint{Date: start.AddDate(0, i, 0), Index: 100, SourceURL: "https://stat.go.jp"}
	}
	return points
}
