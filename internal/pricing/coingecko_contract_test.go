package pricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// Contract tests replay recorded JSON against a local HTTP server (VCR-style fixtures, no network).
// Timeouts are enforced via Resty SetTimeout (strict, sub-second for failure cases).

func loadContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "contracts", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestCoinGeckoClient_ContractReplay_MultiCurrencyRates(t *testing.T) {
	fixture := loadContractFixture(t, "coingecko_simple_price.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/simple/price" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("ids") != "bitcoin" {
			t.Errorf("ids=%q", r.URL.Query().Get("ids"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	rc := resty.New().SetTimeout(2 * time.Second).SetRetryCount(0)
	cg := &CoinGeckoClient{HTTPClient: rc, BaseURL: srv.URL}

	out, err := cg.GetMultiCurrencyRatesIn(context.Background(), []string{"usd", "eur"})
	if err != nil {
		t.Fatal(err)
	}
	btc, ok := out["bitcoin"].(map[string]interface{})
	if !ok {
		t.Fatalf("bitcoin: %#v", out)
	}
	if btc["usd"] != 50000.5 {
		t.Fatalf("usd: %v", btc["usd"])
	}
}

func TestCoinGeckoClient_ContractReplay_GetBTCUSD(t *testing.T) {
	fixture := loadContractFixture(t, "coingecko_simple_price.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	rc := resty.New().SetTimeout(2 * time.Second).SetRetryCount(0)
	cg := &CoinGeckoClient{HTTPClient: rc, BaseURL: srv.URL}

	v, err := cg.GetBTCUSD(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != 50000.5 {
		t.Fatalf("GetBTCUSD = %v", v)
	}
}

func TestCoinGeckoClient_StrictTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bitcoin":{"usd":1}}`))
	}))
	defer srv.Close()

	rc := resty.New().SetTimeout(50 * time.Millisecond).SetRetryCount(0)
	cg := &CoinGeckoClient{HTTPClient: rc, BaseURL: srv.URL}

	_, err := cg.GetMultiCurrencyRatesIn(context.Background(), []string{"usd"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
