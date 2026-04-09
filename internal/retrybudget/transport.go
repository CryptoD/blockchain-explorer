package retrybudget

import (
	"fmt"
	"net/http"
)

// WrapRoundTripper enforces [WithAttemptBudget] on each outbound request. If the context has no budget,
// the base transport runs unchanged.
func WrapRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &budgetTransport{base: base}
}

type budgetTransport struct {
	base http.RoundTripper
}

func (t *budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return t.base.RoundTrip(req)
	}
	b := budgetFrom(req.Context())
	if b == nil {
		return t.base.RoundTrip(req)
	}
	if !tryTake(b) {
		return nil, fmt.Errorf("%w", ErrBudgetExhausted)
	}
	return t.base.RoundTrip(req)
}
