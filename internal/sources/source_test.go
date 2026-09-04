package sources

import (
	"context"
	"errors"
	"io"
	"net/http"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPayPayPublicSourceFetchFunds(t *testing.T) {
	data := `[{"brand":"Fund","brand_url":"https://example.test/fund","corporate":"Manager","nisa_seichou":"on","nisa_tumitate":""}]`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != PayPayFundDataURL || r.Header.Get("User-Agent") == "" {
			t.Fatalf("unexpected request: %s, %q", r.URL, r.Header.Get("User-Agent"))
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(data)), Header: make(http.Header)}, nil
	})}
	items, err := NewPayPayPublicSource(client).FetchFunds(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "Fund" || items[0].NISATsumitate || !items[0].NISAGrowth {
		t.Fatalf("items = %#v, err = %v", items, err)
	}
}

func TestPayPayPublicSourceFetchErrors(t *testing.T) {
	tests := []struct {
		name string
		fn   roundTripFunc
	}{
		{"transport", func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }},
		{"status", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Status: "503 Service Unavailable", Body: io.NopCloser(strings.NewReader(""))}, nil
		}},
		{"invalid json", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader("{"))}, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPayPayPublicSource(&http.Client{Transport: tt.fn}).FetchFunds(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := (&PayPayPublicSource{client: http.DefaultClient, endpoint: "%"}).FetchFunds(context.Background()); err == nil {
		t.Fatal("expected invalid endpoint error")
	}
}

func TestParsePayPayFundListValidationAndDeduplication(t *testing.T) {
	if _, err := parsePayPayFundList(strings.NewReader("[]"), time.Now()); err == nil {
		t.Fatal("expected empty catalogue error")
	}
	if _, err := parsePayPayFundList(strings.NewReader("not-json"), time.Now()); err == nil {
		t.Fatal("expected json error")
	}
	items, err := parsePayPayFundList(strings.NewReader(`[{"brand":""},{"brand":"Fund"},{"brand":"Fund"}]`), time.Now())
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %#v, err = %v", items, err)
	}
}
