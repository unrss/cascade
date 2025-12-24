package shell

import "strings"

// BashEscape escapes a string for safe use in bash double quotes.
// Handles: backslashes, double quotes, dollar signs, backticks, newlines.
func BashEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 10) // Pre-allocate with some headroom for escapes

	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '$':
			b.WriteString(`\$`)
		case '`':
			b.WriteString("\\`")
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}
