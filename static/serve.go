package static

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
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

	var info *FileInfo
	var reader io.Reader
	if c.Content != nil {
		info, reader = c.Content.Get(p)
	}
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

	// determine if there's a hash we need: query/filename 'wins' over content
	var effectiveHash string
	var notModified bool
	cacheForever := false

	if !serve404 {
		effectiveHash = info.ContentHash
		if queryHash := GetQueryHash(r.URL.RawQuery); queryHash != "" {
			effectiveHash = queryHash
			cacheForever = true
		} else if fileHash := GetFileHash(r.URL.Path); fileHash != "" {
			effectiveHash = queryHash
			cacheForever = true
		} else if effectiveHash == "" {
			// TODO: calculate hash based on content?
		}

		// write the etag
		if effectiveHash != "" {
			head.Set("ETag", effectiveHash)
			notModified = r.Header.Get("If-None-Match") == effectiveHash

			// we had a url-based-hash, cache forever
			if cacheForever {
				head.Set("Cache-Control", "public, max-age=7776000, immutable")
			}
		}
	}

	if c.UpdateHeader != nil {
		c.UpdateHeader(head, ServeInfo{
			FileInfo:     *info,
			Is404:        serve404,
			IsHtml:       isHtml,
			IsHead:       r.Method == http.MethodHead,
			NotModified:  notModified,
			CacheForever: cacheForever,
		})
	}

	// short-circuit if etag matched
	if notModified {
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

	// add Server-Timing header to report hash
	if effectiveHash != "" && isHtml {
		b, _ := json.Marshal(effectiveHash)
		if len(b) != 0 {
			w.Header().Add("Server-Timing", fmt.Sprintf("$h;desc=%s", string(b)))
		}
	}

	_, err := io.Copy(w, reader)
	if err != nil {
		log.Printf("couldn't write bytes: p=%v %v", p, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// add secret HTML comment for hash validation
	if c.InsertHtmlHash && effectiveHash != "" && isHtml {
		fmt.Fprintf(w, "<!--:%s:-->\n", effectiveHash)
	}
}
