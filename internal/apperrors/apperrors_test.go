package apperrors

import (
	"errors"
	"testing"
)

func TestErrNotFoundSentinel(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("ErrNotFound identity")
	}
	w := Wrap(ErrNotFound, CodeNotFound, 404, "gone")
	if !errors.Is(w, ErrNotFound) {
		t.Fatal("wrap should unwrap to ErrNotFound")
	}
}

func TestError_Unwrap(t *testing.T) {
	inner := errors.New("db")
	e := Wrap(inner, CodeInternal, 500, "failed")
	if !errors.Is(e, inner) {
		t.Fatal("expected inner")
	}
}
