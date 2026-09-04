package sources

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"paypay-nisa-analyzer/internal/domain"
)

// ParseAdjustedNAVCSV 僅接受容易稽核的 date,adjusted_nav 格式。adjusted_nav
// 必須是已將稅前配息再投資的淨值序列，不能用一般淨值代替。
func ParseAdjustedNAVCSV(r io.Reader, fundID, sourceURL string) ([]domain.PricePoint, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("讀取 CSV 標頭：%w", err)
	}
	dateColumn, navColumn := -1, -1
	for index, header := range headers {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "date":
			dateColumn = index
		case "adjusted_nav":
			navColumn = index
		}
	}
	if dateColumn < 0 || navColumn < 0 {
		return nil, fmt.Errorf("CSV 必須包含 date 與 adjusted_nav 欄位")
	}
	points := []domain.PricePoint{}
	line := 1
	for {
		line++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV 第 %d 行：%w", line, err)
		}
		if len(record) != len(headers) {
			return nil, fmt.Errorf("CSV 第 %d 行欄位數不正確；含逗號的數值必須加上雙引號", line)
		}
		date, err := time.Parse("2006-01-02", strings.TrimSpace(record[dateColumn]))
		if err != nil {
			return nil, fmt.Errorf("CSV 第 %d 行日期無效：%w", line, err)
		}
		nav, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(record[navColumn]), ",", ""), 64)
		if err != nil || nav <= 0 {
			return nil, fmt.Errorf("CSV 第 %d 行 adjusted_nav 無效", line)
		}
		points = append(points, domain.PricePoint{FundID: fundID, Date: date, NAV: nav, SourceURL: sourceURL})
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("CSV 沒有資料列")
	}
	return points, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
