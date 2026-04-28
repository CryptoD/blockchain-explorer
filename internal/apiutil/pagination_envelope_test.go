package apiutil

import "testing"

func TestNewListPagination_HasMore(t *testing.T) {
	p := Pagination{Page: 1, PageSize: 10, Offset: 0, Limit: 10}
	m := NewListPagination(p, 25, 10)
	if m.Total != 25 || !m.HasMore || m.Page != 1 || m.PageSize != 10 {
		t.Fatalf("first page: %#v", m)
	}
	p2 := Pagination{Page: 3, PageSize: 10, Offset: 20, Limit: 10}
	m2 := NewListPagination(p2, 25, 5)
	if m2.HasMore {
		t.Fatalf("last partial page should not has_more: %#v", m2)
	}
	m3 := NewListPagination(Pagination{Page: 1, PageSize: 20, Offset: 0, Limit: 20}, 0, 0)
	if m3.HasMore || m3.Total != 0 {
		t.Fatalf("empty total: %#v", m3)
	}
}

func TestNewFeedPagination(t *testing.T) {
	m := NewFeedPagination(50, 50)
	if !m.HasMore || m.Total != 50 || m.Page != 1 || m.PageSize != 50 {
		t.Fatalf("%#v", m)
	}
	m2 := NewFeedPagination(50, 49)
	if m2.HasMore {
		t.Fatalf("partial batch: %#v", m2)
	}
}
