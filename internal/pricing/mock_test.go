package pricing_test

import (
	"context"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/pricing"
)

func TestMockClient_GetMultiCurrencyRatesIn(t *testing.T) {
	m := &pricing.MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"bitcoin": map[string]interface{}{"usd": 50000.0},
			}, nil
		},
		GetBTCUSDFunc: func(ctx context.Context) (float64, error) {
			return 50000, nil
		},
	}
	rates, err := m.GetMultiCurrencyRatesIn(context.Background(), []string{"usd"})
	if err != nil {
		t.Fatal(err)
	}
	btc, ok := rates["bitcoin"].(map[string]interface{})
	if !ok || btc["usd"] != 50000.0 {
		t.Fatalf("rates = %#v", rates)
	}
	usd, err := m.GetBTCUSD(context.Background())
	if err != nil || usd != 50000 {
		t.Fatalf("GetBTCUSD = %v, %v", usd, err)
	}
}

func TestMockClient_UnsetFunc(t *testing.T) {
	m := &pricing.MockClient{}
	_, err := m.GetMultiCurrencyRatesIn(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when func not set")
	}
}

func TestCompositePricer_MockCrypto(t *testing.T) {
	p := &pricing.CompositePricer{
		Crypto: &pricing.MockCryptoPriceFetcher{
			GetCryptoPriceInFiatFunc: func(ctx context.Context, coinID, fiat string) (float64, bool) {
				if coinID == "bitcoin" && fiat == "usd" {
					return 42000, true
				}
				return 0, false
			},
		},
		Commodity: &pricing.StaticCommoditySource{},
		Bond:      &pricing.StaticBondSource{PricePer100: pricing.DefaultBondPrices()},
	}
	v, ok := p.GetAssetPriceInFiat(context.Background(), pricing.AssetClassCrypto, "btc", "usd", 1)
	if !ok || v != 42000 {
		t.Fatalf("got %v, %v", v, ok)
	}
}

func TestMockCryptoPriceFetcher_NilFunc(t *testing.T) {
	var m *pricing.MockCryptoPriceFetcher
	v, ok := m.GetCryptoPriceInFiat(context.Background(), "bitcoin", "usd")
	if ok || v != 0 {
		t.Fatalf("got %v, %v", v, ok)
	}
}
