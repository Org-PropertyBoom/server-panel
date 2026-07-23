package render

import (
	"regexp"
	"sort"
	"strings"
)

var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// ValidHeaderName reports whether name is a safe HTTP header name for a Caddy
// `header` directive: letters, digits, hyphens only.
func ValidHeaderName(name string) bool {
	return headerNameRe.MatchString(name)
}

// ValidHeaderValue reports whether value is safe inside a quoted Caddy header
// directive — no newlines, quotes, backslash, closing brace, or control chars that
// could break the site block or inject directives.
func ValidHeaderValue(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch r {
		case '\n', '\r', '"', '}', '\\':
			return false
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// HeaderDirectives renders response headers as sorted, one-line, 4-space-indented
// Caddy `header <Name> "<Value>"` directives (each ending in \n), for insertion
// before reverse_proxy inside a site block. Invalid names/values are skipped —
// defensive; they are rejected at write time. Sorted so the render is idempotent.
func HeaderDirectives(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	names := make([]string, 0, len(headers))
	for n, v := range headers {
		if ValidHeaderName(n) && ValidHeaderValue(v) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString("    header " + n + " \"" + headers[n] + "\"\n")
	}
	return b.String()
}
