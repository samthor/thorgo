package static

import (
	"regexp"
)

var (
	// vite includes _ in the hashes
	reHash     = regexp.MustCompile(`([_a-z0-9A-Z]{6,24})`)
	reFileHash = regexp.MustCompile(`(-|\.)([_a-z0-9A-Z]{6,24})\.`)
)

// GetQueryHash matches a complete input to whether it looks like a long-term hash.
// This matches only if the ENTIRE rawQuery is the regexp; anything with a = is ignored.
func GetQueryHash(rawQuery string) string {
	match := reHash.FindStringIndex(rawQuery)
	if match == nil {
		return ""
	}
	if match[0] == 0 && match[1] == len(rawQuery) {
		return rawQuery
	}
	return ""
}

// GetFileHash looks for a hash as a suffix to a file (e.g, "foo-JK1llaO.js").
// This looks for a suffix starting with '-' or '.', but not the extension.
func GetFileHash(filename string) string {
	out := reFileHash.FindStringSubmatch(filename)
	if out == nil {
		return ""
	}
	return out[2]
}
