package news

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// TheNewsAPIProvider implements Provider using https://www.thenewsapi.com/ API.
// Auth is done via the api_token query parameter.
type TheNewsAPIProvider struct {
	BaseURL string
	Token   string
	Client  *resty.Client

	// Optional filters.
	DefaultLanguage   string
	DefaultLocale     string
	DefaultCategories string
}

func (p *TheNewsAPIProvider) Name() string { return "thenewsapi" }

type theNewsAPIArticle struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	ImageURL    string `json:"image_url"`
	Source      string `json:"source"`
	PublishedAt string `json:"published_at"`
}

type theNewsAPIResponse struct {
	Data []theNewsAPIArticle `json:"data"`
}

func (p *TheNewsAPIProvider) Fetch(ctx context.Context, query string, limit int) ([]Article, error) {
	if p == nil {
		return nil, fmt.Errorf("thenewsapi provider is nil")
	}
	if strings.TrimSpace(p.Token) == "" {
		return nil, fmt.Errorf("THENEWSAPI_API_TOKEN is not set")
	}
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base == "" {
		base = "https://api.thenewsapi.com"
	}
	if p.Client == nil {
		p.Client = resty.New().SetTimeout(10 * time.Second).SetRetryCount(2)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Use /v1/news/all for search queries.
	url := base + "/v1/news/all"
	params := map[string]string{
		"api_token": p.Token,
		"search":    query,
		"limit":     fmt.Sprintf("%d", limit),
		"sort":      "published_at",
	}
	if v := strings.TrimSpace(p.DefaultLanguage); v != "" {
		params["language"] = v
	}
	if v := strings.TrimSpace(p.DefaultLocale); v != "" {
		params["locale"] = v
	}
	if v := strings.TrimSpace(p.DefaultCategories); v != "" {
		params["categories"] = v
	}
	resp, err := p.Client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetQueryParams(params).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("thenewsapi request failed: %w", err)
	}

	if resp.StatusCode() == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("thenewsapi unexpected status %d", resp.StatusCode())
	}

	var parsed theNewsAPIResponse
	if err := json.Unmarshal(resp.Body(), &parsed); err != nil {
		return nil, fmt.Errorf("thenewsapi decode failed: %w", err)
	}

	out := make([]Article, 0, len(parsed.Data))
	for _, a := range parsed.Data {
		t := strings.TrimSpace(a.Title)
		u := strings.TrimSpace(a.URL)
		s := strings.TrimSpace(a.Source)
		if t == "" || u == "" || s == "" {
			continue
		}
		published := parseProviderTime(a.PublishedAt)
		if published.IsZero() {
			published = time.Now().UTC()
		}
		out = append(out, Article{
			Headline:    t,
			Summary:     strings.TrimSpace(a.Description),
			Source:      s,
			URL:         u,
			ImageURL:    strings.TrimSpace(a.ImageURL),
			PublishedAt: published,
		})
	}
	return out, nil
}

func parseProviderTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// TheNewsAPI commonly returns RFC3339 with fractional seconds.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	// Last resort: try without timezone (treat as UTC).
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
