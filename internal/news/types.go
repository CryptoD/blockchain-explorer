package news

import "time"

// Article is the normalized shape returned by the news service.
// Keep this stable for frontend consumption.
type Article struct {
	Headline    string    `json:"headline"`
	Summary     string    `json:"summary,omitempty"`
	Source      string    `json:"source"`
	URL         string    `json:"url"`
	ImageURL    string    `json:"image_url,omitempty"`
	PublishedAt time.Time `json:"published_at"`

	// Provider-specific fields (not exported in JSON) can be added later if needed.
}

// ListResponse is returned by the API handlers.
type ListResponse struct {
	Data []Article `json:"data"`
	Meta Meta      `json:"meta"`
}

type Meta struct {
	Provider string `json:"provider"`
	Cached   bool   `json:"cached"`
	Stale    bool   `json:"stale"`
	Query    string `json:"query,omitempty"`
}
