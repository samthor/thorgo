package static

import (
	"io"
	"net/http"
)

// FileInfo is returned from Content.
type FileInfo struct {
	// Any available hash from the file's contents, or CRC or something.
	ContentHash string

	// ContentType if known.
	ContentType string

	// Additional headers to include, e.g, for CSP
	Header http.Header
}

// ServeInfo is passed to UpdateHeader to control header generation.
type ServeInfo struct {
	FileInfo
	Is404        bool
	IsHtml       bool
	IsHead       bool
	NotModified  bool // did the client match the prior ETag
	CacheForever bool
}

// Content controls what is rendered inside ServeFs.
type Content interface {
	Get(path string) (*FileInfo, io.ReadCloser)
	Exists(path string) bool
}

// ServeFs implements http.Handler.
type ServeFs struct {
	Content Content

	// QueryHash controls whether the query is searched for a hash-like structure (to mark immutable).
	QueryHash bool

	// AddPrefix is added to all paths before being checked inside Content.
	AddPrefix string

	// Normally, no files "foo.html" are served. This allows them to be served at "foo", without a trailing slash.
	// If "foo.html" AND "foo/index.html" exist, "/foo" will load the former, and "/foo/" will load the latter.
	ServeNakedHtml bool

	// AllowFrame controls the X-Frame-Options header.
	AllowFrame bool

	// HtmlNotFoundPath is loaded from Content if we think this is a missing page (with a trailing slash).
	// It does not have AddPrefix applied to it (in case it's "outside" normal serving code).
	HtmlNotFoundPath string

	// SpaMode will search upwards from a missing probably-HTML page for an index (or correctly named, if ServeNakedHtml is enabled) file and serve this with a 200 response.
	SpaMode bool

	// InsertHtmlHash controls whether a short `<!--:<hash>:-->` is added to each served HTML page.
	InsertHtmlHash bool

	// UpdateHeader may be provided to update the headers of returned responses. Useful for CSP.
	UpdateHeader func(http.Header, ServeInfo)
}
