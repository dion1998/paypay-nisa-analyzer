package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

var emaxisFundCode = regexp.MustCompile(`/fund/([0-9]{6})\.html`)

// FetchEMAXISReinvestedNAV 使用發行商公開的配息再投資圖表資料，
// 不會讀取或登入任何 PayPay 帳戶。
func FetchEMAXISReinvestedNAV(ctx context.Context, client *http.Client, fund domain.Fund) ([]domain.PricePoint, bool, error) {
	pageURL, err := url.Parse(fund.HistoryURL)
	if err != nil || pageURL.Host == "" || !strings.HasSuffix(strings.ToLower(pageURL.Host), "emaxis.jp") {
		return nil, false, nil
	}
	match := emaxisFundCode.FindStringSubmatch(pageURL.Path)
	if len(match) != 2 {
		return nil, false, nil
	}
	chartURL := "https://emaxis.jp/fund_file/chart/chart_data_" + match[1] + ".js"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chartURL, nil)
	if err != nil {
		return nil, true, err
	}
	req.Header.Set("User-Agent", "PayPayNISAAnalyzer/1.0 (local personal research tool)")
	response, err := client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, true, fmt.Errorf("eMAXIS history returned %s", response.Status)
	}
	var payload struct {
		Rows []struct {
			Date string  `json:"BASE_DATE"`
			NAV  float64 `json:"REINVEST_BASE_PRICE"`
		} `json:"ROWS"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, true, fmt.Errorf("parse eMAXIS history: %w", err)
	}
	points := make([]domain.PricePoint, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		date, err := time.Parse("20060102", row.Date)
		if err == nil && row.NAV > 0 {
			points = append(points, domain.PricePoint{FundID: fund.ID, Date: date.UTC(), NAV: row.NAV, SourceURL: chartURL})
		}
	}
	if len(points) == 0 {
		return nil, true, fmt.Errorf("eMAXIS returned no usable NAV points")
	}
	return points, true, nil
}
