// Package news implements contextual article fetching: provider contracts, caching, and Service.
// Business rules and provider wiring live here; HTTP handlers compose the service in internal/server.
// The cmd/server binary must not import this package directly; only internal/server wires it.
package news
