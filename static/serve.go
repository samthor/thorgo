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

	var isHtml bool

	serve404 := false

	var endsWithSlash bool
	p := c.AddPrefix + r.URL.Path
	if strings.HasSuffix(p, ".html") {
		// don't support direct loading
		serve404 = true
	} else if strings.HasSuffix(p, "/") {
		// ... but we _look_ for "/index.html"
		p += "index.html"
		endsWithSlash = true
	}
	p = strings.TrimPrefix(p, "/")

	head := w.Header()

	var info *FileInfo
	var reader io.ReadCloser
	if !serve404 && c.Content != nil {
		// guard reading content if we had a "bad url" (i.e., ends with "/index.html")
		info, reader = c.Content.Get(p)
	}
	if info == nil && !endsWithSlash {
		if c.ServeNakedHtml {
			// we have "foo.html", serve directly
			info, reader = c.Content.Get(p + ".html")
			if info != nil {
				isHtml = true
			}
		}
		// if we don't have "foo.html", look for "foo/index.html"
		if info == nil {
			checkP := strings.TrimPrefix(p+"/index.html", "/")

			indexExists := c.Content.Exists(checkP)
			if indexExists {
				// we know that the next call will add "index.html" to this
				head.Set("Location", p+"/")
				w.WriteHeader(http.StatusSeeOther)
				return
			}
		}
	}

	if info == nil {
		ext := filepath.Ext(r.URL.Path) // original ext
		if c.ServeNakedHtml {
			if ext != "" {
				// not inferred as a HTML page, "blah.css" or even "test.html"
				w.WriteHeader(http.StatusNotFound)
				return
			}
		} else if !endsWithSlash {
			// not inferred as a HTML page (needs slash)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// search upwards for any maching files
		if c.SpaMode {
			curr := p
			for {
				update := filepath.Dir(curr)
				if update == curr {
					break // can't go further
				}
				curr = update // this will be _without slash_

				checkIndex := curr + "/index.html"
				if checkIndex == "./index.html" {
					// filePath.Dir of "randomString" is always ".", so check for it
					checkIndex = "index.html"
				}
				info, reader = c.Content.Get(checkIndex)
				if info != nil {
					isHtml = true
					break
				}

				if curr != "." && c.ServeNakedHtml {
					info, reader = c.Content.Get(curr + ".html")
					if info != nil {
						isHtml = true
						break
					}
				}
			}
		}

		if info == nil {
			serve404 = true
			if c.HtmlNotFoundPath != "" {
				info, reader = c.Content.Get(c.HtmlNotFoundPath)
				if info != nil {
					isHtml = true
				}
			}
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
		if ct == "" && isHtml {
			ct = "text/html"
		}
	}
	if ct != "" {
		head.Set("Content-Type", ct)
		if !isHtml {
			isHtml = ct == "text/html" || strings.HasPrefix(ct, "text/html;")
		}
	}

	// determine if there's a hash we need: query/filename 'wins' over content
	var effectiveHash string
	var notModified bool
	cacheForeverForUrl := false

	if !serve404 {
		effectiveHash = info.ContentHash

		if fileHash := GetFileHash(r.URL.Path); fileHash != "" {
			effectiveHash = fileHash
			cacheForeverForUrl = true
		}

		if effectiveHash == "" && c.QueryHash {
			if queryHash := GetQueryHash(r.URL.RawQuery); queryHash != "" {
				effectiveHash = queryHash
				cacheForeverForUrl = true
			}
		}

		if effectiveHash == "" {
			// TODO: calculate hash based on content?
		}

		// write the etag
		if effectiveHash != "" {
			head.Set("ETag", effectiveHash)
			notModified = r.Header.Get("If-None-Match") == effectiveHash

			// we had a url-based-hash, cache forever
			if cacheForeverForUrl {
				head.Set("Cache-Control", "public, max-age=7776000, immutable")
			}

			// add Server-Timing header to report hash (readable in JS)
			if isHtml {
				b, _ := json.Marshal(effectiveHash)
				if len(b) != 0 {
					head.Add("Server-Timing", fmt.Sprintf("$h;desc=%s", string(b)))
				}
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
			CacheForever: cacheForeverForUrl,
		})
		if etag := head.Get("ETag"); etag != "" && r.Header.Get("If-None-Match") == etag {
			// maybe UpdateHeader set ETag
			notModified = true
		}
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

	defer reader.Close()
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
