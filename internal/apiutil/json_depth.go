package apiutil

import (
	"encoding/json"
	"errors"
	"io"
)

// ErrJSONTooDeep is returned when JSON object/array nesting exceeds the configured maximum.
var ErrJSONTooDeep = errors.New("json: nesting too deep")

// ValidateJSONDepth streams JSON from r and returns ErrJSONTooDeep if nesting of objects and arrays
// exceeds maxDepth. maxDepth <= 0 disables the check (returns nil if JSON is valid).
// Other errors from the decoder (invalid JSON) are returned as-is.
func ValidateJSONDepth(r io.Reader, maxDepth int) error {
	if maxDepth <= 0 {
		return nil
	}
	dec := json.NewDecoder(r)
	depth := 0
	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch d := t.(type) {
		case json.Delim:
			switch d {
			case '{', '[':
				depth++
				if depth > maxDepth {
					return ErrJSONTooDeep
				}
			case '}', ']':
				if depth > 0 {
					depth--
				}
			}
		}
	}
}
