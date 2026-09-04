package sources

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

const StatisticsBureauCPIURL = "https://www.stat.go.jp/data/cpi/2020/csv/zmi2020aa.csv"

type CPISource interface {
	FetchCPI(context.Context) ([]domain.CPIPoint, error)
}

// StatisticsBureauCPISource 讀取日本總務省公開的全國 CPI 月資料。
type StatisticsBureauCPISource struct{ Client *http.Client }

func (s StatisticsBureauCPISource) FetchCPI(ctx context.Context) ([]domain.CPIPoint, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, StatisticsBureauCPIURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Statistics Bureau CPI returned %s", resp.Status)
	}
	reader := csv.NewReader(io.LimitReader(resp.Body, 8<<20))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	points := []domain.CPIPoint{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse CPI CSV: %w", err)
		}
		if len(record) < 2 {
			continue
		}
		month := strings.TrimSpace(record[0])
		if len(month) != 6 {
			continue
		}
		date, err := time.Parse("200601", month)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(record[1]), ",", ""), 64)
		if err != nil || value <= 0 {
			continue
		}
		points = append(points, domain.CPIPoint{Date: date.UTC(), Index: value, SourceURL: StatisticsBureauCPIURL})
	}
	if len(points) < 24 {
		return nil, errors.New("CPI 公開序列不足 24 個月")
	}
	return points, nil
}
