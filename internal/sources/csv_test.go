package sources

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseAdjustedNAVCSV(t *testing.T) {
	points, err := ParseAdjustedNAVCSV(strings.NewReader("date,adjusted_nav\n2021-01-29,10,123\n"), "fund", "https://issuer.example/report")
	if err == nil {
		t.Fatal("expected invalid CSV because the NAV value needs quoting")
	}
	points, err = ParseAdjustedNAVCSV(strings.NewReader("date,adjusted_nav\n2021-01-29,10123\n"), "fund", "https://issuer.example/report")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].NAV != 10123 {
		t.Fatalf("points = %#v", points)
	}
}

func TestParseAdjustedNAVCSVValidation(t *testing.T) {
	tests := []struct {
		name string
		csv  string
	}{
		{"missing columns", "date,nav\n2026-01-01,100\n"},
		{"empty", "date,adjusted_nav\n"},
		{"bad date", "date,adjusted_nav\nnope,100\n"},
		{"bad nav", "date,adjusted_nav\n2026-01-01,0\n"},
		{"wrong field count", "date,adjusted_nav\n2026-01-01\n"},
		{"csv parser error", "date,adjusted_nav\n\"unterminated\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseAdjustedNAVCSV(strings.NewReader(tt.csv), "fund", "source"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	points, err := ParseAdjustedNAVCSV(strings.NewReader(" adjusted_nav , ignored , DATE \n\"1,234.50\",x,2026-01-01\n"), "fund", "source")
	if err != nil || len(points) != 1 || points[0].NAV != 1234.5 {
		t.Fatalf("points = %#v, err = %v", points, err)
	}
	if max(5, 4) != 5 || max(4, 5) != 5 {
		t.Fatal("max returned wrong value")
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestParseAdjustedNAVCSVReadError(t *testing.T) {
	if _, err := ParseAdjustedNAVCSV(failingReader{errors.New("boom")}, "fund", "source"); err == nil {
		t.Fatal("expected header read error")
	}
	var _ io.Reader = failingReader{}
}
