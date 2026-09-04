package domain

import (
	"errors"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"time"
)

const (
	forecastMonths         = 60
	minimumHistoryMonths   = 12
	minimumHoldingSamples  = 24
	bootstrapPaths         = 1000
	realSuccessThreshold   = .80
	successLowerThreshold  = .70
	p10RealThreshold       = -.20
	es10Threshold          = -.30
	drawdownThreshold      = -.45
	requiredStableHorizons = 3
)

var ErrInsufficientHistory = errors.New("至少需要 12 個月的可靠歷史淨值資料才能推估")

// Calculate 優先使用實際觀察到的五年歷史區間；基金歷史不足五年時，改採固定
// 隨機種子的三個月區塊自助法，保留報酬連續波動的特性。
func Calculate(fund Fund, raw []PricePoint, rawCPI []CPIPoint, initial, monthly float64) (Analysis, error) {
	if initial < 0 || monthly < 0 || math.IsNaN(initial) || math.IsNaN(monthly) || math.IsInf(initial, 0) || math.IsInf(monthly, 0) {
		return Analysis{}, errors.New("本金與每月投入必須是非負的有限數字")
	}
	points := monthlyPoints(raw)
	if len(points) < minimumHistoryMonths+1 {
		return Analysis{}, ErrInsufficientHistory
	}
	paths, observed := forecastPaths(fund.ID, points, initial, monthly)
	criteria, recommended, holdingSamples, positiveRate := holdingRecommendation(fund.ID, points, rawCPI, initial, monthly)
	p10, p50, p90 := percentile(paths, .10), percentile(paths, .50), percentile(paths, .90)
	return Analysis{Fund: fund, InitialAmount: initial, MonthlyAmount: monthly, TotalContributions: initial + monthly*forecastMonths, P10: p10, P50: p50, P90: p90,
		Scenarios: []Scenario{{"保守 P10", p10}, {"中位 P50", p50}, {"樂觀 P90", p90}}, SampleCount: len(paths), HistoryStart: points[0].Date, HistoryEnd: points[len(points)-1].Date, MaxDrawdown: maxDrawdown(points),
		RecommendedYears: recommended, HoldingSampleCount: holdingSamples, PositiveReturnRate: positiveRate, HoldingCriteria: criteria, NISANote: nisaNote(fund, initial, monthly), DataAsOf: points[len(points)-1].Date,
		Methodology: forecastMethodology(observed), Disclaimer: "歷史情境不代表未來結果；本工具不構成投資、稅務或買賣建議。"}, nil
}
func forecastMethodology(observed bool) string {
	if observed {
		return "以每段可用的連續 60 個月歷史淨值報酬，逐一套用本金與每月底定投後取 P10／P50／P90。"
	}
	return "歷史未滿 5 年：以可用月報酬的連續三個月區塊、固定亂數種子重抽 1,000 條 5 年路徑，再套用本金與每月底定投。"
}
func monthlyPoints(raw []PricePoint) []PricePoint {
	points := append([]PricePoint(nil), raw...)
	sort.Slice(points, func(i, j int) bool { return points[i].Date.Before(points[j].Date) })
	result := []PricePoint{}
	for _, p := range points {
		if p.NAV <= 0 || p.Date.IsZero() {
			continue
		}
		key := p.Date.UTC().Format("2006-01")
		if len(result) > 0 && result[len(result)-1].Date.UTC().Format("2006-01") == key {
			result[len(result)-1] = p
		} else {
			result = append(result, p)
		}
	}
	return result
}
func forecastPaths(fundID string, points []PricePoint, initial, monthly float64) ([]float64, bool) {
	if len(points) >= forecastMonths+1 {
		paths := []float64{}
		for start := 0; start+forecastMonths < len(points); start++ {
			paths = append(paths, cashFlowValue(points[start:start+forecastMonths+1], initial, monthly))
		}
		sort.Float64s(paths)
		return paths, true
	}
	returns := returnsOf(points)
	paths := make([]float64, 0, bootstrapPaths)
	rng := deterministicRNG(fundID, points[len(points)-1].Date, forecastMonths)
	for i := 0; i < bootstrapPaths; i++ {
		value := initial
		for month := 0; month < forecastMonths; {
			start := rng.Intn(len(returns))
			for offset := 0; offset < 3 && month < forecastMonths; offset++ {
				value *= 1 + returns[(start+offset)%len(returns)]
				value += monthly
				month++
			}
		}
		paths = append(paths, value)
	}
	sort.Float64s(paths)
	return paths, false
}
func cashFlowValue(points []PricePoint, initial, monthly float64) float64 {
	value := initial
	for month := 1; month < len(points); month++ {
		value *= points[month].NAV / points[month-1].NAV
		value += monthly
	}
	return value
}
func returnsOf(points []PricePoint) []float64 {
	result := make([]float64, 0, len(points)-1)
	for i := 1; i < len(points); i++ {
		result = append(result, points[i].NAV/points[i-1].NAV-1)
	}
	return result
}

type holdingOutcome struct{ realReturn, drawdown float64 }

func holdingRecommendation(fundID string, points []PricePoint, rawCPI []CPIPoint, initial, monthly float64) (HoldingCriteria, int, int, float64) {
	base := defaultCriteria(rawCPI)
	cpi := monthlyCPI(rawCPI)
	if len(cpi) < 24 {
		base.FailedReasons = []string{"無法取得足夠的日本全國 CPI 月度資料，不能產生實質持有年限建議"}
		return base, 0, 0, 0
	}
	var last HoldingCriteria
	stable := 0
	for years := 1; years <= 20; years++ {
		outcomes, observed, boot := historicalHoldingOutcomes(fundID, points, cpi, years*12, initial, monthly)
		if len(outcomes) == 0 {
			break
		}
		criteria := criteriaFor(years, outcomes, observed, boot, cpi[len(cpi)-1].Date)
		if criteria.RiskCriteriaPassed {
			stable++
		} else {
			stable = 0
		}
		criteria.StableHorizonsPassed = stable
		criteria.Passed = stable >= requiredStableHorizons
		if criteria.Passed {
			return criteria, years, criteria.SampleCount, criteria.RealSuccessRate
		}
		last = criteria
	}
	if last.Years == 0 {
		last = base
	}
	return last, 0, last.SampleCount, last.RealSuccessRate
}
func historicalHoldingOutcomes(fundID string, points []PricePoint, cpi []CPIPoint, months int, initial, monthly float64) ([]holdingOutcome, int, bool) {
	byMonth := map[string]float64{}
	for _, p := range cpi {
		if p.Index > 0 {
			byMonth[p.Date.UTC().Format("2006-01")] = p.Index
		}
	}
	result := []holdingOutcome{}
	for start := 0; start+months < len(points); start++ {
		startCPI, okStart := byMonth[points[start].Date.UTC().Format("2006-01")]
		endCPI, okEnd := byMonth[points[start+months].Date.UTC().Format("2006-01")]
		if !okStart || !okEnd {
			continue
		}
		value := cashFlowValue(points[start:start+months+1], initial, monthly)
		contributions := initial + monthly*float64(months)
		result = append(result, holdingOutcome{(value/endCPI)/(contributions/startCPI) - 1, maxDrawdown(points[start : start+months+1])})
	}
	observed := len(result)
	if observed >= minimumHoldingSamples {
		return result, observed, false
	}
	changes := pairedChanges(points, byMonth)
	if len(changes) < 6 {
		return result, observed, false
	}
	rng := deterministicRNG(fundID, points[len(points)-1].Date, months)
	for path := len(result); path < bootstrapPaths; path++ {
		value, contributions, index, peak, worst := initial, initial, 100.0, initial, 0.0
		for month := 0; month < months; {
			start := rng.Intn(len(changes))
			for offset := 0; offset < 3 && month < months; offset++ {
				change := changes[(start+offset)%len(changes)]
				value *= 1 + change.fund
				value += monthly
				contributions += monthly
				index *= 1 + change.inflation
				if value > peak {
					peak = value
				}
				if dd := value/peak - 1; dd < worst {
					worst = dd
				}
				month++
			}
		}
		result = append(result, holdingOutcome{value/(contributions*index/100) - 1, worst})
	}
	return result, observed, true
}

type pairedChange struct{ fund, inflation float64 }

func pairedChanges(points []PricePoint, cpi map[string]float64) []pairedChange {
	result := []pairedChange{}
	for i := 1; i < len(points); i++ {
		prior, okPrior := cpi[points[i-1].Date.UTC().Format("2006-01")]
		current, okCurrent := cpi[points[i].Date.UTC().Format("2006-01")]
		if okPrior && okCurrent && prior > 0 {
			result = append(result, pairedChange{points[i].NAV/points[i-1].NAV - 1, current/prior - 1})
		}
	}
	return result
}
func criteriaFor(years int, outcomes []holdingOutcome, observed int, boot bool, cpiAsOf time.Time) HoldingCriteria {
	returns := make([]float64, len(outcomes))
	wins, worst := 0, 0.0
	for i, o := range outcomes {
		returns[i] = o.realReturn
		if o.realReturn > 0 {
			wins++
		}
		if o.drawdown < worst {
			worst = o.drawdown
		}
	}
	sort.Float64s(returns)
	c := defaultCriteria(nil)
	c.Years, c.SampleCount, c.ObservedSampleCount = years, len(outcomes), observed
	c.UsesBootstrap, c.CPIAvailable, c.CPIAsOf = boot, true, cpiAsOf
	c.EvidenceLevel = "歷史滾動樣本"
	if boot {
		c.EvidenceLevel = "歷史不足；以三個月區塊 bootstrap 補足情境"
		c.EffectiveBlockCount = 3
	}
	c.RealSuccessRate = float64(wins) / float64(len(outcomes))
	c.SuccessRateLowerBound = wilsonLowerBound(wins, len(outcomes))
	c.P10RealReturn = percentile(returns, .10)
	c.ExpectedShortfall10 = expectedShortfall(returns, .10)
	c.MaximumDrawdown, c.WorstPathDrawdown = percentileDrawdown(outcomes, .10), worst
	c.RiskCriteriaPassed = c.RealSuccessRate >= realSuccessThreshold && c.SuccessRateLowerBound >= successLowerThreshold && c.P10RealReturn >= p10RealThreshold && c.ExpectedShortfall10 >= es10Threshold && c.MaximumDrawdown >= drawdownThreshold
	if c.RealSuccessRate < realSuccessThreshold {
		c.FailedReasons = append(c.FailedReasons, "實質正報酬率未達 80%")
	}
	if c.SuccessRateLowerBound < successLowerThreshold {
		c.FailedReasons = append(c.FailedReasons, "成功率的保守下限未達 70%")
	}
	if c.P10RealReturn < p10RealThreshold {
		c.FailedReasons = append(c.FailedReasons, "P10 實質期末報酬低於 -20%")
	}
	if c.ExpectedShortfall10 < es10Threshold {
		c.FailedReasons = append(c.FailedReasons, "最差 10% 情境平均值低於 -30%")
	}
	if c.MaximumDrawdown < drawdownThreshold {
		c.FailedReasons = append(c.FailedReasons, "P10 路徑最大回撤低於 -45%")
	}
	return c
}
func defaultCriteria(cpi []CPIPoint) HoldingCriteria {
	c := HoldingCriteria{SuccessRateThreshold: realSuccessThreshold, SuccessLowerBoundThreshold: successLowerThreshold, P10Threshold: p10RealThreshold, ExpectedShortfallThreshold: es10Threshold, MaximumDrawdownThreshold: drawdownThreshold, RequiredStableHorizons: requiredStableHorizons}
	if len(cpi) > 0 {
		c.CPIAsOf = cpi[len(cpi)-1].Date
	}
	return c
}
func monthlyCPI(raw []CPIPoint) []CPIPoint {
	points := append([]CPIPoint(nil), raw...)
	sort.Slice(points, func(i, j int) bool { return points[i].Date.Before(points[j].Date) })
	result := []CPIPoint{}
	for _, p := range points {
		if p.Index <= 0 || p.Date.IsZero() {
			continue
		}
		key := p.Date.UTC().Format("2006-01")
		if len(result) > 0 && result[len(result)-1].Date.UTC().Format("2006-01") == key {
			result[len(result)-1] = p
		} else {
			result = append(result, p)
		}
	}
	return result
}
func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	if p <= 0 {
		return v[0]
	}
	if p >= 1 {
		return v[len(v)-1]
	}
	position := p * float64(len(v)-1)
	lo, hi := int(math.Floor(position)), int(math.Ceil(position))
	if lo == hi {
		return v[lo]
	}
	return v[lo] + (v[hi]-v[lo])*(position-float64(lo))
}
func expectedShortfall(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	n := int(math.Ceil(float64(len(v)) * p))
	if n < 1 {
		n = 1
	}
	sum := 0.0
	for _, x := range v[:n] {
		sum += x
	}
	return sum / float64(n)
}
func maxDrawdown(points []PricePoint) float64 {
	if len(points) == 0 {
		return 0
	}
	peak, worst := points[0].NAV, 0.0
	for _, p := range points {
		if p.NAV > peak {
			peak = p.NAV
		}
		if dd := p.NAV/peak - 1; dd < worst {
			worst = dd
		}
	}
	return worst
}
func percentileDrawdown(outcomes []holdingOutcome, p float64) float64 {
	v := make([]float64, len(outcomes))
	for i, o := range outcomes {
		v[i] = o.drawdown
	}
	sort.Float64s(v)
	return percentile(v, p)
}
func wilsonLowerBound(wins, total int) float64 {
	if total == 0 {
		return 0
	}
	z := 1.2815515655446004
	n := float64(total)
	phat := float64(wins) / n
	return (phat + z*z/(2*n) - z*math.Sqrt((phat*(1-phat)+z*z/(4*n))/n)) / (1 + z*z/n)
}
func deterministicRNG(fundID string, date time.Time, months int) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fundID + date.UTC().Format(time.RFC3339) + strconv.Itoa(months)))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}
func nisaNote(f Fund, initial, monthly float64) string {
	firstYear := initial + monthly*12
	eligibility := "此基金的 NISA 類別尚未確認。"
	if f.NISATsumitate && f.NISAGrowth {
		eligibility = "此基金列為 NISA 累積投資枠及成長投資枠適用。"
	} else if f.NISATsumitate {
		eligibility = "此基金列為 NISA 累積投資枠適用。"
	} else if f.NISAGrowth {
		eligibility = "此基金列為 NISA 成長投資枠適用。"
	}
	return eligibility + " 依本次輸入，首 12 個月投入約為 ¥" + formatYen(firstYear) + "；實際可用額度仍須以帳戶狀態為準。"
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
