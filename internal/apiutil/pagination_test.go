package apiutil

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParsePagination_Table(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		query        string
		defaultSize  int
		maxSize      int
		wantPage     int
		wantPageSize int
		wantOffset   int
	}{
		{
			name: "defaults_when_empty", query: "",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 20, wantOffset: 0,
		},
		{
			name: "explicit_page_and_size", query: "page=2&page_size=10",
			defaultSize: 20, maxSize: 100,
			wantPage: 2, wantPageSize: 10, wantOffset: 10,
		},
		{
			name: "page_below_one_clamped", query: "page=0&page_size=5",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 5, wantOffset: 0,
		},
		{
			name: "negative_page_clamped", query: "page=-3&page_size=5",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 5, wantOffset: 0,
		},
		{
			name: "page_size_zero_uses_default", query: "page=1&page_size=0",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 20, wantOffset: 0,
		},
		{
			name: "page_size_negative_uses_default", query: "page=1&page_size=-1",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 20, wantOffset: 0,
		},
		{
			name: "page_size_capped_to_max", query: "page=1&page_size=500",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 100, wantOffset: 0,
		},
		{
			name: "large_page_computes_offset", query: "page=3&page_size=15",
			defaultSize: 20, maxSize: 100,
			wantPage: 3, wantPageSize: 15, wantOffset: 30,
		},
		{
			name: "non_numeric_page_ignored", query: "page=abc&page_size=12",
			defaultSize: 20, maxSize: 100,
			wantPage: 1, wantPageSize: 12, wantOffset: 0,
		},
		{
			name: "non_numeric_page_size_ignored", query: "page=2&page_size=xy",
			defaultSize: 20, maxSize: 100,
			wantPage: 2, wantPageSize: 20, wantOffset: 20,
		},
		{
			name: "custom_defaults", query: "",
			defaultSize: 50, maxSize: 200,
			wantPage: 1, wantPageSize: 50, wantOffset: 0,
		},
		{
			name: "page_capped_at_maxPage", query: "page=2000000&page_size=10",
			defaultSize: 20, maxSize: 100,
			wantPage: 1_000_000, wantPageSize: 10, wantOffset: (1_000_000 - 1) * 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			path := "/"
			if tt.query != "" {
				path = "/?" + tt.query
			}
			c.Request = httptest.NewRequest("GET", path, nil)

			got := ParsePagination(c, tt.defaultSize, tt.maxSize)
			if got.Page != tt.wantPage {
				t.Fatalf("Page: got %d want %d", got.Page, tt.wantPage)
			}
			if got.PageSize != tt.wantPageSize {
				t.Fatalf("PageSize: got %d want %d", got.PageSize, tt.wantPageSize)
			}
			if got.Offset != tt.wantOffset {
				t.Fatalf("Offset: got %d want %d", got.Offset, tt.wantOffset)
			}
			if got.Limit != got.PageSize {
				t.Fatalf("Limit should match PageSize: %d vs %d", got.Limit, got.PageSize)
			}
		})
	}
}
