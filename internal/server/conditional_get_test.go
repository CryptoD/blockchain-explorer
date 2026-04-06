package server

import "testing"

func TestIfNoneMatchEquals(t *testing.T) {
	etag := "\"deadbeef\""
	if !ifNoneMatchEquals("\"deadbeef\"", etag) {
		t.Fatal("exact match")
	}
	if !ifNoneMatchEquals("  \"deadbeef\"  ", etag) {
		t.Fatal("trimmed match")
	}
	if !ifNoneMatchEquals("\"a\", \"deadbeef\"", etag) {
		t.Fatal("list match")
	}
	if ifNoneMatchEquals("*", etag) {
		t.Fatal("star must not match (unsafe for first GET)")
	}
	if ifNoneMatchEquals("", etag) {
		t.Fatal("empty header")
	}
	if ifNoneMatchEquals("\"other\"", etag) {
		t.Fatal("mismatch")
	}
}
