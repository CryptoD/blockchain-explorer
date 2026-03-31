package repos

import "testing"

func TestPortfolioKey(t *testing.T) {
	if got := PortfolioKey("alice", "p1"); got != "portfolio:alice:p1" {
		t.Fatalf("got %q", got)
	}
}

func TestWatchlistKey(t *testing.T) {
	if got := WatchlistKey("bob", "w1"); got != "watchlist:bob:w1" {
		t.Fatalf("got %q", got)
	}
}

func TestUsernameFromUserKey(t *testing.T) {
	u, ok := UsernameFromUserKey("user:carol")
	if !ok || u != "carol" {
		t.Fatalf("ok=%v u=%q", ok, u)
	}
	if _, ok := UsernameFromUserKey("portfolio:x:y"); ok {
		t.Fatal("expected false")
	}
}
