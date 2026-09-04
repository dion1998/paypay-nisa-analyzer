package domain

import "time"

type Fund struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Manager        string    `json:"manager"`
	NISATsumitate  bool      `json:"nisaTsumitate"`
	NISAGrowth     bool      `json:"nisaGrowth"`
	PayPayURL      string    `json:"paypayUrl"`
	HistoryURL     string    `json:"historyUrl,omitempty"`
	TrustFeeRate   *float64  `json:"trustFeeRate,omitempty"`
	TrustFeeSource string    `json:"trustFeeSource,omitempty"`
	RefreshedAt    time.Time `json:"refreshedAt"`
}

type PricePoint struct {
	FundID    string    `json:"fundId"`
	Date      time.Time `json:"date"`
	NAV       float64   `json:"nav"`
	SourceURL string    `json:"sourceUrl"`
}
type CPIPoint struct {
	Date      time.Time `json:"date"`
	Index     float64   `json:"index"`
	SourceURL string    `json:"sourceUrl"`
}
type Insight struct {
	ID          int64     `json:"id"`
	FundID      string    `json:"fundId"`
	Publisher   string    `json:"publisher"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	SourceURL   string    `json:"sourceUrl"`
	PublishedAt time.Time `json:"publishedAt"`
}
type AnalysisRequest struct {
	FundID        string  `json:"fundId"`
	InitialAmount float64 `json:"initialAmount"`
	MonthlyAmount float64 `json:"monthlyAmount"`
}
type Scenario struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

// HoldingCriteria 列出建議持有年限判定所使用的每一項規則與門檻。
type HoldingCriteria struct {
	Years                      int       `json:"years"`
	SampleCount                int       `json:"sampleCount"`
	ObservedSampleCount        int       `json:"observedSampleCount"`
	EffectiveBlockCount        int       `json:"effectiveBlockCount"`
	UsesBootstrap              bool      `json:"usesBootstrap"`
	EvidenceLevel              string    `json:"evidenceLevel"`
	CPIAsOf                    time.Time `json:"cpiAsOf"`
	CPIAvailable               bool      `json:"cpiAvailable"`
	RealSuccessRate            float64   `json:"realSuccessRate"`
	SuccessRateLowerBound      float64   `json:"successRateLowerBound"`
	P10RealReturn              float64   `json:"p10RealReturn"`
	ExpectedShortfall10        float64   `json:"expectedShortfall10"`
	MaximumDrawdown            float64   `json:"maximumDrawdown"`
	WorstPathDrawdown          float64   `json:"worstPathDrawdown"`
	SuccessRateThreshold       float64   `json:"successRateThreshold"`
	SuccessLowerBoundThreshold float64   `json:"successLowerBoundThreshold"`
	P10Threshold               float64   `json:"p10Threshold"`
	ExpectedShortfallThreshold float64   `json:"expectedShortfallThreshold"`
	MaximumDrawdownThreshold   float64   `json:"maximumDrawdownThreshold"`
	RequiredStableHorizons     int       `json:"requiredStableHorizons"`
	StableHorizonsPassed       int       `json:"stableHorizonsPassed"`
	RiskCriteriaPassed         bool      `json:"riskCriteriaPassed"`
	Passed                     bool      `json:"passed"`
	FailedReasons              []string  `json:"failedReasons"`
}

type Analysis struct {
	Fund               Fund            `json:"fund"`
	InitialAmount      float64         `json:"initialAmount"`
	MonthlyAmount      float64         `json:"monthlyAmount"`
	TotalContributions float64         `json:"totalContributions"`
	P10                float64         `json:"p10"`
	P50                float64         `json:"p50"`
	P90                float64         `json:"p90"`
	Scenarios          []Scenario      `json:"scenarios"`
	SampleCount        int             `json:"sampleCount"`
	HistoryStart       time.Time       `json:"historyStart"`
	HistoryEnd         time.Time       `json:"historyEnd"`
	MaxDrawdown        float64         `json:"maxDrawdown"`
	RecommendedYears   int             `json:"recommendedYears"`
	HoldingSampleCount int             `json:"holdingSampleCount"`
	PositiveReturnRate float64         `json:"positiveReturnRate"`
	HoldingCriteria    HoldingCriteria `json:"holdingCriteria"`
	NISANote           string          `json:"nisaNote"`
	DataAsOf           time.Time       `json:"dataAsOf"`
	Methodology        string          `json:"methodology"`
	Disclaimer         string          `json:"disclaimer"`
}
