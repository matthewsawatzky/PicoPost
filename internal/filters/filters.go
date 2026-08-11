// Package filters implements the simple server-side content filters.
//
// Filters run before a post is stored. Matching is deliberately simple:
// case-insensitive substring matching on normalized text. The public
// error is always the same ("post_rejected") so clients cannot learn
// which rule triggered.
package filters

import (
	"net/url"
	"strings"
)

// Filter is a compiled deny-list for one field.
type Filter struct {
	deny []string
}

// New compiles a deny list. Entries are lowercased and trimmed.
func New(deny []string) *Filter {
	out := make([]string, 0, len(deny))
	for _, d := range deny {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			out = append(out, d)
		}
	}
	return &Filter{deny: out}
}

// Rejects reports whether value contains any denied term.
// Comparison is case-insensitive on trimmed input.
func (f *Filter) Rejects(value string) bool {
	if f == nil || len(f.deny) == 0 {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(value))
	for _, d := range f.deny {
		if strings.Contains(v, d) {
			return true
		}
	}
	return false
}

// CountURLs returns the number of URLs found in text. A URL is any
// http:// or https:// scheme token.
func CountURLs(text string) int {
	count := 0
	for _, tok := range strings.Fields(text) {
		u, err := url.Parse(tok)
		if err != nil {
			continue
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			count++
		}
	}
	return count
}
