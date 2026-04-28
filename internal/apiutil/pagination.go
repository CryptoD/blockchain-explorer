package apiutil

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// Standard list pagination defaults for JSON list endpoints (query: page, page_size).
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	// MaxPageSizeNews caps news feeds backed by external providers.
	MaxPageSizeNews = 50
)

// Pagination captures common pagination parameters for list endpoints.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Offset   int `json:"offset"`
	Limit    int `json:"limit"`
}

// Sort captures generic sorting parameters.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" or "desc"
}

// ParsePagination parses "page" and "page_size" query parameters, enforcing sane
// defaults and maximums. It never returns a page size greater than maxPageSize
// and treats any invalid values as the defaults.
func ParsePagination(c *gin.Context, defaultPageSize, maxPageSize int) Pagination {
	if defaultPageSize <= 0 {
		defaultPageSize = 20
	}
	if maxPageSize <= 0 {
		maxPageSize = 100
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	const maxPage = 1_000_000
	if page > maxPage {
		page = maxPage
	}

	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	offset64 := int64(page-1) * int64(pageSize)
	maxInt := int64(^uint(0) >> 1)
	if offset64 < 0 || offset64 > maxInt {
		page = 1
		pageSize = defaultPageSize
		offset64 = 0
	}
	offset := int(offset64)

	return Pagination{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
		Limit:    pageSize,
	}
}

// ListPagination is the standard JSON pagination object for list endpoints (same shape everywhere).
type ListPagination struct {
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
	HasMore  bool `json:"has_more"`
}

// NewListPagination builds metadata for offset/limit slicing: total is the full filtered count,
// returned is the length of the current page slice. has_more means more items exist after this page.
func NewListPagination(p Pagination, total, returned int) ListPagination {
	hasMore := total > 0 && p.Offset+returned < total
	return ListPagination{
		Total:    total,
		Page:     p.Page,
		PageSize: p.PageSize,
		HasMore:  hasMore,
	}
}

// NewFeedPagination builds metadata when the upstream returns at most limit items without a stable
// total across pages (provider-backed feeds). Total equals returnedLen; has_more is true iff the
// response is full vs the requested limit (may indicate more upstream results — not authoritative).
func NewFeedPagination(limit, returnedLen int) ListPagination {
	return ListPagination{
		Total:    returnedLen,
		Page:     1,
		PageSize: limit,
		HasMore:  limit > 0 && returnedLen == limit,
	}
}

// ParseSort parses "sort_by" and "sort_dir" query parameters, applying a
// default field/direction and restricting to a set of allowed fields.
func ParseSort(c *gin.Context, defaultField, defaultDirection string, allowedFields map[string]bool) Sort {
	field := c.DefaultQuery("sort_by", defaultField)
	if allowedFields != nil && !allowedFields[field] {
		field = defaultField
	}

	dir := c.DefaultQuery("sort_dir", defaultDirection)
	if dir != "asc" && dir != "desc" {
		dir = defaultDirection
	}

	return Sort{
		Field:     field,
		Direction: dir,
	}
}
