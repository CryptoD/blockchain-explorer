package news

import "context"

// Provider fetches news articles for a query.
type Provider interface {
	Name() string
	Fetch(ctx context.Context, query string, limit int) ([]Article, error)
}
