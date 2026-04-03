package apiutil

// Query param fuzzing for ParsePagination / ParseSort (ROADMAP task 23).
// Run: go test ./internal/apiutil -fuzz=FuzzParsePagination -fuzztime=30s
//
//	go test ./internal/apiutil -fuzz=FuzzParseSort -fuzztime=30s

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func FuzzParsePagination(f *testing.F) {
	f.Add("page=1&page_size=10", 20, 100)
	f.Fuzz(func(t *testing.T, rawQuery string, def int, max int) {
		if def < 1 || def > 10000 {
			def = 20
		}
		if max < 1 || max > 100000 {
			max = 100
		}
		if max < def {
			max = def
		}

		u, err := url.Parse("http://example.com/x")
		if err != nil {
			t.Fatal(err)
		}
		q, err := url.ParseQuery(rawQuery)
		if err != nil {
			return
		}
		u.RawQuery = q.Encode()

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", u.String(), nil)

		p := ParsePagination(c, def, max)
		if p.Page < 1 {
			t.Fatalf("Page=%d", p.Page)
		}
		if p.PageSize < 1 {
			t.Fatalf("PageSize=%d", p.PageSize)
		}
		want := int64(p.Page-1) * int64(p.PageSize)
		if int64(p.Offset) != want {
			t.Fatalf("Offset=%d want %d page=%d size=%d", p.Offset, want, p.Page, p.PageSize)
		}
		if p.Limit != p.PageSize {
			t.Fatalf("Limit=%d PageSize=%d", p.Limit, p.PageSize)
		}
	})
}

func FuzzParseSort(f *testing.F) {
	f.Add("sort_by=created&sort_dir=asc")
	f.Fuzz(func(t *testing.T, rawQuery string) {
		allowed := map[string]bool{"created": true, "updated": true}

		u, err := url.Parse("http://example.com/x")
		if err != nil {
			t.Fatal(err)
		}
		q, err := url.ParseQuery(rawQuery)
		if err != nil {
			return
		}
		u.RawQuery = q.Encode()

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", u.String(), nil)

		s := ParseSort(c, "created", "desc", allowed)
		if s.Field == "" {
			t.Fatal("empty field")
		}
		if s.Direction != "asc" && s.Direction != "desc" {
			t.Fatalf("direction=%q", s.Direction)
		}
	})
}
