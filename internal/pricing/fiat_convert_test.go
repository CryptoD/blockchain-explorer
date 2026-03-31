package pricing

import "testing"

func TestUSDPerUnitOfFiatFromBTCRates(t *testing.T) {
	rates := map[string]interface{}{
		"bitcoin": map[string]interface{}{
			"usd": 50000.0,
			"eur": 40000.0,
		},
	}
	v, ok := USDPerUnitOfFiatFromBTCRates(rates, "eur")
	if !ok {
		t.Fatal("expected ok")
	}
	want := 50000.0 / 40000.0
	if v != want {
		t.Fatalf("got %v want %v", v, want)
	}
	v2, ok2 := USDPerUnitOfFiatFromBTCRates(rates, "usd")
	if !ok2 || v2 != 1 {
		t.Fatalf("usd: %v, %v", v2, ok2)
	}
}

func TestUSDPerUnitOfFiatFromBTCRates_invalid(t *testing.T) {
	_, ok := USDPerUnitOfFiatFromBTCRates(map[string]interface{}{}, "eur")
	if ok {
		t.Fatal("expected false")
	}
}
