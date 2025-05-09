package rope

import (
	"unicode/utf16"
)

// JSLength returns the JS length of the given string (which in Go, is always UTF-8).
// We don't care about raw bytes because we probably only have this string because we got it _from_ JSON, which is UTF-8.
func JSLength(s string) (count int) {
	for _, r := range s {
		each := utf16.RuneLen(r)

		if each < 0 {
			count++
		} else {
			count += each
		}
	}

	return count
}
