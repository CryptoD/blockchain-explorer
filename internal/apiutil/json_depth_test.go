package apiutil

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateJSONDepth_OK(t *testing.T) {
	const max = 8
	cases := []string{
		`{}`,
		`[]`,
		`{"a":1}`,
		`{"a":{"b":{"c":1}}}`,
		`"plain"`,
	}
	for _, s := range cases {
		if err := ValidateJSONDepth(strings.NewReader(s), max); err != nil {
			t.Fatalf("max=%d body=%q: %v", max, s, err)
		}
	}
}

func TestValidateJSONDepth_TooDeep(t *testing.T) {
	// depth 4: { { { { } } } } as nested objects
	s := `{"a":{"b":{"c":{"d":true}}}}`
	if err := ValidateJSONDepth(strings.NewReader(s), 3); err == nil {
		t.Fatal("expected error for depth 4 with max 3")
	} else if !errors.Is(err, ErrJSONTooDeep) {
		t.Fatalf("want ErrJSONTooDeep, got %v", err)
	}
}

func TestValidateJSONDepth_Disabled(t *testing.T) {
	s := `{"a":{"b":{"c":{"d":{"e":1}}}}}`
	if err := ValidateJSONDepth(strings.NewReader(s), 0); err != nil {
		t.Fatal(err)
	}
}
