// Package naming normalizes metric and tag names into stable, low-cardinality
// identifiers (lowercase, non-alphanumeric runs collapsed to underscores).
package naming

import "strings"

// Normalize lowercases s and replaces every run of non-alphanumeric characters
// with a single underscore, trimming leading/trailing underscores. For example
// "Answer Relevancy" becomes "answer_relevancy" and "exact_match,strict-match"
// becomes "exact_match_strict_match".
func Normalize(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
