package shell

import "strings"

// FishEscape escapes a string for safe use in fish shell single quotes.
// Fish single quotes are mostly literal, but require escaping:
// - Single quotes: ' -> \'
// - Backslashes before quotes: \ -> \\
//
// Fish also interprets backslash-newline as line continuation in single quotes,
// so we escape those sequences as well.
func FishEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 10) // Pre-allocate with some headroom for escapes

	for _, r := range s {
		switch r {
		case '\'':
			b.WriteString(`\'`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}
