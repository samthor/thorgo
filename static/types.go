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

	// AddPrefix is added to all paths before being checked inside Content.
	AddPrefix string

	// AllowFrame controls the X-Frame-Options header.
	AllowFrame bool

	// HtmlNotFoundPath is loaded from Content if we think this is a missing page (with a trailing slash). It does not have AddPrefix applied to it.
	HtmlNotFoundPath string

	// InsertHtmlHash controls whether a short `<!--:<hash>:-->` is added to each served HTML page.
	InsertHtmlHash bool

	// UpdateHeader may be provided to update the headers of returned responses. Useful for CSP.
	UpdateHeader func(http.Header, ServeInfo)
}
