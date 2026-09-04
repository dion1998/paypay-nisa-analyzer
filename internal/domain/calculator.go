package domain

import (
	"errors"
	"math"
	"sort"
	"strconv"
)

const forecastMonths = 60

var ErrInsufficientHistory = errors.New("至少需要 60 個月的可靠歷史淨值資料才能推估")

// Calculate applies every available historical 60-month return path to the same
// cash flow. NAV must already be adjusted for distributions reinvested before tax.
func Calculate(fund Fund, raw []PricePoint, initial, monthly float64) (Analysis, error) {
	if initial < 0 || monthly < 0 || math.IsNaN(initial) || math.IsNaN(monthly) || math.IsInf(initial, 0) || math.IsInf(monthly, 0) {
		return Analysis{}, errors.New("本金與每月投入必須是零或正數")
	}
	points := monthlyPoints(raw)
	if len(points) < forecastMonths+1 {
		return Analysis{}, ErrInsufficientHistory
	}

	paths := make([]float64, 0, len(points)-forecastMonths)
	for start := 0; start+forecastMonths < len(points); start++ {
		value := initial
		for month := 1; month <= forecastMonths; month++ {
			value *= points[start+month].NAV / points[start+month-1].NAV
			value += monthly // End-of-month contribution.
		}
		paths = append(paths, value)
	}
	sort.Float64s(paths)

	recommended, holdingSamples, positiveRate := holdingRecommendation(points, initial, monthly)
	note := nisaNote(fund, initial, monthly)
	return Analysis{
		Fund:               fund,
		InitialAmount:      initial,
		MonthlyAmount:      monthly,
		TotalContributions: initial + monthly*forecastMonths,
		P10:                percentile(paths, .10),
		P50:                percentile(paths, .50),
		P90:                percentile(paths, .90),
		Scenarios:          []Scenario{{Label: "保守歷史情境（P10）", Amount: percentile(paths, .10)}, {Label: "歷史中位情境（P50）", Amount: percentile(paths, .50)}, {Label: "樂觀歷史情境（P90）", Amount: percentile(paths, .90)}},
		SampleCount:        len(paths),
		HistoryStart:       points[0].Date,
		HistoryEnd:         points[len(points)-1].Date,
		MaxDrawdown:        maxDrawdown(points),
		RecommendedYears:   recommended,
		HoldingSampleCount: holdingSamples,
		PositiveReturnRate: positiveRate,
		NISANote:           note,
		DataAsOf:           points[len(points)-1].Date,
		Methodology:        "將每個可用的歷史 60 個月報酬路徑套用到本金與每月底投入；P10、P50、P90 是這些歷史情境的百分位數。",
		Disclaimer:         "歷史績效不代表未來結果。本工具不構成投資、稅務或買賣建議。",
	}, nil
}

func monthlyPoints(raw []PricePoint) []PricePoint {
	points := append([]PricePoint(nil), raw...)
	sort.Slice(points, func(i, j int) bool { return points[i].Date.Before(points[j].Date) })
	result := make([]PricePoint, 0, len(points))
	for _, point := range points {
		if point.NAV <= 0 || point.Date.IsZero() {
			continue
		}
		key := point.Date.Format("2006-01")
		if len(result) > 0 && result[len(result)-1].Date.Format("2006-01") == key {
			result[len(result)-1] = point // Last available NAV in the calendar month.
		} else {
			result = append(result, point)
		}
	}
	return result
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 1 {
		return values[len(values)-1]
	}
	position := p * float64(len(values)-1)
	low, high := int(math.Floor(position)), int(math.Ceil(position))
	if low == high {
		return values[low]
	}
	return values[low] + (values[high]-values[low])*(position-float64(low))
}

func maxDrawdown(points []PricePoint) float64 {
	peak, worst := points[0].NAV, 0.0
	for _, point := range points {
		if point.NAV > peak {
			peak = point.NAV
		}
		drawdown := (point.NAV - peak) / peak
		if drawdown < worst {
			worst = drawdown
		}
	}
	return worst
}

func holdingRecommendation(points []PricePoint, initial, monthly float64) (years, samples int, rate float64) {
	maxYears := min(20, (len(points)-1)/12)
	for year := 1; year <= maxYears; year++ {
		months := year * 12
		wins, total := 0, 0
		for start := 0; start+months < len(points); start++ {
			value := initial
			for month := 1; month <= months; month++ {
				value *= points[start+month].NAV / points[start+month-1].NAV
				value += monthly
			}
			if value > initial+monthly*float64(months) {
				wins++
			}
			total++
		}
		if total >= 24 {
			currentRate := float64(wins) / float64(total)
			if currentRate >= .80 {
				return year, total, currentRate
			}
		}
	}
	return 0, 0, 0
}

func nisaNote(f Fund, initial, monthly float64) string {
	firstTwelveMonths := initial + monthly*12
	eligibility := "此基金不在已同步的 NISA 對象資料中。"
	if f.NISATsumitate && f.NISAGrowth {
		eligibility = "此基金標示為つみたて投資枠與成長投資枠對象。"
	} else if f.NISATsumitate {
		eligibility = "此基金標示為つみたて投資枠對象。"
	} else if f.NISAGrowth {
		eligibility = "此基金標示為成長投資枠對象。"
	}
	return eligibility + " 前 12 個月計畫投入約 ¥" + formatYen(firstTwelveMonths) + "；實際可用額度仍須扣除其他 NISA 投資，且制度與額度可能變更。"
}

func formatYen(v float64) string { return strconvFormat(int64(math.Round(v))) }
func strconvFormat(v int64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if negative {
		return "-" + s
	}
	return s
}
