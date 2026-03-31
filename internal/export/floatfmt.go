package export

import "strconv"

// FormatFloat renders a float for tables and PDF cells (compact scientific for huge/tiny values).
func FormatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	if f >= 1e6 || (f < 0.0001 && f > 0) {
		return strconv.FormatFloat(f, 'e', 2, 64)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}
