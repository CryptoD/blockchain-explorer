// Package retrybudget caps total outbound HTTP attempts per logical scope (e.g. one inbound request)
// when context is propagated to Resty. See also OUTBOUND_HTTP_RETRY_COUNT for Resty-layer retries.
package retrybudget

import (
	"context"
	"errors"
	"sync/atomic"
)

type ctxKey struct{}

// ErrBudgetExhausted is returned by the transport wrapper when no outbound attempts remain for this context.
var ErrBudgetExhausted = errors.New("outbound HTTP attempt budget exhausted")

type budget struct {
	left atomic.Int32
}

// WithAttemptBudget attaches a maximum number of outbound HTTP RoundTrip calls for this context tree.
// maxAttempts <= 0 means no budget is enforced (unlimited for contexts using this package's transport).
func WithAttemptBudget(ctx context.Context, maxAttempts int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxAttempts <= 0 {
		return ctx
	}
	b := &budget{}
	b.left.Store(int32(maxAttempts))
	return context.WithValue(ctx, ctxKey{}, b)
}

func budgetFrom(ctx context.Context) *budget {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(ctxKey{}).(*budget)
	return v
}

func tryTake(b *budget) bool {
	if b == nil {
		return true
	}
	for {
		cur := b.left.Load()
		if cur <= 0 {
			return false
		}
		if b.left.CompareAndSwap(cur, cur-1) {
			return true
		}
	}
}
