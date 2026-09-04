package sources

import (
	"strings"
	"testing"
	"time"
)

func TestParsePayPayFundList(t *testing.T) {
	data := `[{"brand":"eMAXIS Slim 米国株式","brand_url":"https://issuer.example/fund","corporate":"三菱UFJ","nisa_seichou":"on","nisa_tumitate":"on"}]`
	funds, err := parsePayPayFundList(strings.NewReader(data), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(funds) != 1 {
		t.Fatalf("funds = %d", len(funds))
	}
	if !funds[0].NISATsumitate || !funds[0].NISAGrowth {
		t.Fatalf("unexpected NISA flags: %#v", funds[0])
	}
}
