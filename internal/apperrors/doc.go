// Package apperrors defines stable API error codes and typed errors for handlers.
// Hot paths should return [ErrNotFound] or [*Error] instead of fmt.Errorf strings alone.
package apperrors
