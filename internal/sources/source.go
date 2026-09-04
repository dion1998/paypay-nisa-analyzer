package sources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

const PayPayFundDataURL = "https://www.paypay-sec.co.jp/fund/list/data-fund.json"

type FundCatalogSource interface {
	FetchFunds(context.Context) ([]domain.Fund, error)
}

// PayPayPublicSource 僅取得 PayPay 公開的基金清單 JSON，不使用登入中的瀏覽器
// 工作階段或私有 API。公開格式改變時，更新會回傳錯誤並保留既有快取資料。
type PayPayPublicSource struct {
	client   *http.Client
	endpoint string
}

func NewPayPayPublicSource(client *http.Client) *PayPayPublicSource {
	return &PayPayPublicSource{client: client, endpoint: PayPayFundDataURL}
}

func (s *PayPayPublicSource) FetchFunds(ctx context.Context) ([]domain.Fund, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PayPayNISAAnalyzer/1.0 (local personal research tool)")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PayPay 公開清單回應 %s", response.Status)
	}
	return parsePayPayFundList(response.Body, time.Now().UTC())
}

type payPayFundRow struct {
	Brand         string `json:"brand"`
	BrandURL      string `json:"brand_url"`
	Corporate     string `json:"corporate"`
	NISAGrowth    string `json:"nisa_seichou"`
	NISATsumitate string `json:"nisa_tumitate"`
}

func parsePayPayFundList(body io.Reader, refreshed time.Time) ([]domain.Fund, error) {
	var rows []payPayFundRow
	if err := json.NewDecoder(body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("解析 PayPay 公開基金 JSON：%w", err)
	}
	seen, funds := map[string]bool{}, []domain.Fund{}
	for _, row := range rows {
		if row.Brand == "" || seen[row.Brand] {
			continue
		}
		seen[row.Brand] = true
		hash := fmt.Sprintf("paypay-%x", sha256.Sum256([]byte(row.Brand)))[:24]
		funds = append(funds, domain.Fund{ID: hash, Name: row.Brand, Manager: row.Corporate, NISATsumitate: row.NISATsumitate == "on", NISAGrowth: row.NISAGrowth == "on", PayPayURL: row.BrandURL, HistoryURL: row.BrandURL, RefreshedAt: refreshed})
	}
	if len(funds) == 0 {
		return nil, fmt.Errorf("PayPay 清單格式已變更或未找到基金；未覆寫本機快取")
	}
	return funds, nil
}
