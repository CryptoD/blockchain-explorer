package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/pricing"
)

func TestEvaluatePriceAlerts_MGetBatchAndTrigger(t *testing.T) {
	resetCache()
	SetPricingClient(&pricing.MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"bitcoin": map[string]interface{}{"usd": 50000.0},
			}, nil
		},
	})
	defer SetPricingClient(nil)

	now := time.Now().UTC()
	// Above 40k — should trigger at 50k spot.
	a := PriceAlert{
		ID:             "t1",
		Username:       "alice",
		Symbol:         "bitcoin",
		Currency:       "usd",
		Threshold:      40000,
		Direction:      "above",
		DeliveryMethod: "in_app",
		IsActive:       true,
		Created:        now,
		Updated:        now,
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	key := priceAlertKeyPrefix + "alice:t1"
	if err := rdb.Set(ctx, key, raw, 0).Err(); err != nil {
		t.Fatal(err)
	}

	evaluatePriceAlerts()

	got, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	var out PriceAlert
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatal(err)
	}
	if out.IsActive {
		t.Fatal("expected alert deactivated after trigger")
	}
	if out.TriggeredAt == nil {
		t.Fatal("expected TriggeredAt set")
	}
}

func TestEvaluatePriceAlerts_InactiveSkipped(t *testing.T) {
	resetCache()
	SetPricingClient(&pricing.MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"bitcoin": map[string]interface{}{"usd": 50000.0},
			}, nil
		},
	})
	defer SetPricingClient(nil)

	now := time.Now().UTC()
	a := PriceAlert{
		ID:             "i1",
		Username:       "bob",
		Symbol:         "bitcoin",
		Currency:       "usd",
		Threshold:      40000,
		Direction:      "above",
		DeliveryMethod: "in_app",
		IsActive:       false,
		Created:        now,
		Updated:        now,
	}
	raw, _ := json.Marshal(a)
	key := priceAlertKeyPrefix + "bob:i1"
	if err := rdb.Set(ctx, key, raw, 0).Err(); err != nil {
		t.Fatal(err)
	}

	evaluatePriceAlerts()

	got, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	var out PriceAlert
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatal(err)
	}
	if out.IsActive {
		t.Fatal("inactive alert should stay inactive")
	}
	if out.TriggeredAt != nil {
		t.Fatal("inactive alert should not set TriggeredAt")
	}
}
