package static

import (
	"io"
	"net/http"
)

// FileInfo is returned from Content.
type FileInfo struct {
	Hash string

	// Is this Hash purely from the file's contents. If false, and Hash is set, is from the filename.
	ContentHash bool

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
	IfNoneMatch  bool
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

	// UpdateHeader may be provided to update the headers of returned responses. Useful for CSP.
	UpdateHeader func(http.Header, ServeInfo)
}
