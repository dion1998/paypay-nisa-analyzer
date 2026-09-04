package sources

import (
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
