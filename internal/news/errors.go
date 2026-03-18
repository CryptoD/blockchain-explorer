package news

import "errors"

var (
	// ErrRateLimited signals the provider rejected the request due to rate limiting.
	ErrRateLimited = errors.New("news provider rate limited")
)

