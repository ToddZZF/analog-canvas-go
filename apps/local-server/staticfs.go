package main

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// The editor bundle is served with the desktop shell's exact response
// contract: a CSP admitting only bundle scripts plus the document's hashed
// inline bootstrap, immutable caching for hashed assets, no-cache for the
// document, and editor routes answered by index.html. Port of app-protocol.ts.
//
//go:embed all:web
var webRoot embed.FS

type staticHandler struct {
	files           fs.FS
	indexHTML       []byte
	contentSecurity string
	secureHeaders   [][2]string
}

func newStaticHandler() (*staticHandler, error) {
	files, err := fs.Sub(webRoot, "web")
	if err != nil {
		return nil, err
	}
	return newStaticHandlerFromFS(files)
}

func newStaticHandlerFromFS(files fs.FS) (*staticHandler, error) {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, err
	}
	handler := &staticHandler{
		files:     files,
		indexHTML: index,
	}
	handler.contentSecurity = buildContentSecurityPolicy(index)
	handler.secureHeaders = [][2]string{
		{"content-security-policy", handler.contentSecurity},
		{"cross-origin-opener-policy", "same-origin"},
		{"x-content-type-options", "nosniff"},
	}
	return handler, nil
}

// buildContentSecurityPolicy hashes the document's inline bootstrap scripts
// for a script-src policy that admits only those scripts and bundled modules.
func buildContentSecurityPolicy(index []byte) string {
	hashes := inlineScriptHashes(string(index))
	sources := append([]string{"'self'"}, hashes...)
	policy := strings.Join([]string{
		"default-src 'self'",
		"img-src 'self' blob: data:",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self' data:",
		// 'wasm-unsafe-eval' lets the bundled RNL WebAssembly compile and
		// admits no JavaScript eval.
		"script-src " + strings.Join(sources, " ") + " 'wasm-unsafe-eval'",
		"worker-src 'self' blob:",
		// The vector PDF renderer reads its bundled period font from a data
		// URL; this admits embedded bytes, never an external destination.
		"connect-src 'self' data:",
		"frame-src 'none'",
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'none'",
	}, "; ")
	return policy
}

func inlineScriptHashes(document string) []string {
	hashes := []string{}
	remaining := document
	for {
		start := strings.Index(remaining, "<script")
		if start < 0 {
			break
		}
		openEnd := strings.Index(remaining[start:], ">")
		if openEnd < 0 {
			break
		}
		openTag := remaining[start : start+openEnd+1]
		bodyStart := start + openEnd + 1
		closeStart := strings.Index(remaining[bodyStart:], "</script>")
		if closeStart < 0 {
			break
		}
		body := remaining[bodyStart : bodyStart+closeStart]
		remaining = remaining[bodyStart+closeStart+len("</script>"):]
		if strings.Contains(openTag, " src=") {
			continue
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		sum := sha256.Sum256([]byte(body))
		hashes = append(hashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return hashes
}

func contentTypeOf(name string) string {
	extension := path.Ext(name)
	switch extension {
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json", ".webmanifest":
		if extension == ".json" {
			return "application/json; charset=utf-8"
		}
		return "application/manifest+json"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".wasm":
		return "application/wasm"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

func (handler *staticHandler) applySecureHeaders(w http.ResponseWriter) {
	for _, header := range handler.secureHeaders {
		w.Header().Set(header[0], header[1])
	}
}

func (handler *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		handler.applySecureHeaders(w)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	target, err := fs.ReadFile(handler.files, name)
	switch {
	case err == nil:
		// Served.
	case path.Ext(name) != "":
		handler.applySecureHeaders(w)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	default:
		// Extensionless paths are editor routes; the document answers them.
		target = handler.indexHTML
	}
	handler.applySecureHeaders(w)
	if name == "index.html" || string(target) == string(handler.indexHTML) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		w.Header().Set("cache-control", "no-cache")
	} else {
		w.Header().Set("content-type", contentTypeOf(name))
		w.Header().Set("cache-control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("content-length", strconv.Itoa(len(target)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(target)
}
