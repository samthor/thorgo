package static

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reHash = regexp.MustCompile(`([a-z0-9A-Z]{6,24})`)
)

func (c *ServeFs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var endsWithSlash bool
	p := c.AddPrefix + r.URL.Path
	if strings.HasSuffix(p, "/index.html") {
		// don't support direct loading
		w.WriteHeader(http.StatusNotFound)
		return
	}

	head := w.Header()

	if strings.HasSuffix(p, "/") {
		p += "index.html"
		endsWithSlash = true
	}
	p = strings.TrimPrefix(p, "/")

	info, reader := c.Content.Get(p)
	if info == nil && !endsWithSlash {
		checkP := strings.TrimPrefix(p+"/index.html", "/")

		indexExists := c.Content.Exists(checkP)
		if indexExists {
			// we know that the next call will add "index.html" to this
			head.Set("Location", p+"/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
	}

	serve404 := false
	if info == nil {
		if !endsWithSlash {
			// not inferred as a HTML page
			w.WriteHeader(http.StatusNotFound)
			return
		}

		serve404 = true
		if c.HtmlNotFoundPath != "" {
			info, reader = c.Content.Get(c.HtmlNotFoundPath)
		}
		if info == nil {
			// no 404 page available
			info = &FileInfo{}
		}
	}

	// copy headers (do direct, already canonicalized)
	for h, already := range info.Header {
		head[h] = append(head[h], already...)
	}

	// frames (default deny)
	if !c.AllowFrame {
		head.Set("X-Frame-Options", "deny")
	}

	// include Content-Type
	ct := info.ContentType
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(p))
	}
	if ct != "" {
		head.Set("Content-Type", ct)
	}
	isHtml := ct == "text/html" || strings.HasPrefix(ct, "text/html;")

	// include Etag if we have one
	cacheForever := false
	if info.Hash != "" && !serve404 {
		head.Set("ETag", info.Hash)
		if !info.ContentHash {
			cacheForever = true
		}
	}

	// if we have a query-string that looks like a hash only (i.e., no & etc) then cache forever
	if !serve404 && !cacheForever && r.URL.RawQuery != "" && reHash.MatchString(r.URL.RawQuery) {
		cacheForever = true
	}

	if cacheForever {
		head.Set("Cache-Control", "public, max-age=7776000, immutable")
	}

	isNoneMatch := !serve404 && info.Hash != "" && r.Header.Get("If-None-Match") == info.Hash

	if c.UpdateHeader != nil {
		c.UpdateHeader(head, ServeInfo{
			FileInfo:     *info,
			Is404:        serve404,
			IsHtml:       isHtml,
			IsHead:       r.Method == http.MethodHead,
			IfNoneMatch:  isNoneMatch,
			CacheForever: cacheForever,
		})
	}

	// check etag
	if isNoneMatch {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// if 404 (html or not) explicitly write header
	if serve404 {
		w.WriteHeader(http.StatusNotFound)
	}

	if r.Method == "HEAD" || reader == nil {
		return // don't serve bytes
	}

	_, err := io.Copy(w, reader)
	if err != nil {
		log.Printf("couldn't write bytes: p=%v %v", p, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// add etag to html entrypoints
	if info.ContentHash && info.Hash != "" {
		if isHtml {
			fmt.Fprintf(w, "<!--:%s:-->\n", info.Hash)
		}
	}
}
