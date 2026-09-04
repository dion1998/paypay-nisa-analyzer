package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

var nissayFundCode = regexp.MustCompile(`var_fundcode\s*=\s*'([0-9]+)'`)

func FetchNissayReinvestedNAV(ctx context.Context, client *http.Client, fund domain.Fund) ([]domain.PricePoint, bool, error) {
	pageURL, err := url.Parse(fund.HistoryURL)
	if err != nil || !strings.HasSuffix(strings.ToLower(pageURL.Host), "nam.co.jp") || !strings.Contains(pageURL.Path, "/fundinfo/") {
		return nil, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fund.HistoryURL, nil)
	if err != nil {
		return nil, true, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, true, fmt.Errorf("Nissay page returned %s", response.Status)
	}
	page, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, true, err
	}
	match := nissayFundCode.FindStringSubmatch(string(page))
	if len(match) != 2 {
		return nil, true, fmt.Errorf("Nissay fund code was not found")
	}
	chartURL := "https://www.nam.co.jp/fundinfo/data/chart.php?fund_code=" + match[1]
	chartRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, chartURL, nil)
	if err != nil {
		return nil, true, err
	}
	chartResponse, err := client.Do(chartRequest)
	if err != nil {
		return nil, true, err
	}
	defer chartResponse.Body.Close()
	if chartResponse.StatusCode != http.StatusOK {
		return nil, true, fmt.Errorf("Nissay history returned %s", chartResponse.Status)
	}
	var payload []struct {
		Values []struct {
			Date string  `json:"data-date"`
			NAV  float64 `json:"data-plow-back"`
		} `json:"graph-value1"`
	}
	if err := json.NewDecoder(chartResponse.Body).Decode(&payload); err != nil {
		return nil, true, fmt.Errorf("parse Nissay history: %w", err)
	}
	points := []domain.PricePoint{}
	for _, series := range payload {
		for _, row := range series.Values {
			date, err := time.Parse("2006-01-02", row.Date)
			if err == nil && row.NAV > 0 {
				points = append(points, domain.PricePoint{FundID: fund.ID, Date: date.UTC(), NAV: row.NAV, SourceURL: chartURL})
			}
		}
	}
	if len(points) == 0 {
		return nil, true, fmt.Errorf("Nissay returned no usable NAV points")
	}
	return points, true, nil
}
