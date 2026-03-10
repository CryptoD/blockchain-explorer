package apiutil

import (
	"strconv"

	"github.com/gin-gonic/gin"
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

	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	offset := (page - 1) * pageSize

	return Pagination{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
		Limit:    pageSize,
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

