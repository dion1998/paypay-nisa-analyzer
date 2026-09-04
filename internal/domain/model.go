package domain

import "time"

type Fund struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Manager       string    `json:"manager"`
	NISATsumitate bool      `json:"nisaTsumitate"`
	NISAGrowth    bool      `json:"nisaGrowth"`
	PayPayURL     string    `json:"paypayUrl"`
	HistoryURL    string    `json:"historyUrl,omitempty"`
	RefreshedAt   time.Time `json:"refreshedAt"`
}

type PricePoint struct {
	FundID    string    `json:"fundId"`
	Date      time.Time `json:"date"`
	NAV       float64   `json:"nav"`
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

type Analysis struct {
	Fund               Fund       `json:"fund"`
	InitialAmount      float64    `json:"initialAmount"`
	MonthlyAmount      float64    `json:"monthlyAmount"`
	TotalContributions float64    `json:"totalContributions"`
	P10                float64    `json:"p10"`
	P50                float64    `json:"p50"`
	P90                float64    `json:"p90"`
	Scenarios          []Scenario `json:"scenarios"`
	SampleCount        int        `json:"sampleCount"`
	HistoryStart       time.Time  `json:"historyStart"`
	HistoryEnd         time.Time  `json:"historyEnd"`
	MaxDrawdown        float64    `json:"maxDrawdown"`
	RecommendedYears   int        `json:"recommendedYears"`
	HoldingSampleCount int        `json:"holdingSampleCount"`
	PositiveReturnRate float64    `json:"positiveReturnRate"`
	NISANote           string     `json:"nisaNote"`
	DataAsOf           time.Time  `json:"dataAsOf"`
	Methodology        string     `json:"methodology"`
	Disclaimer         string     `json:"disclaimer"`
}
