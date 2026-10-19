package main

import (
	"net/http"
	"strings"
)

// localServer routes the embedded editor bundle and the /desktop host API
// the editor's local composition talks to. Anything under /api/ belongs to
// the hosted cloud product and is deliberately absent here.
type localServer struct {
	static     *staticHandler
	files      *fileHandler
	components *componentHandler
	system     *systemHandler
	external   *externalHandler
	quit       *quitHandler
}

func (server *localServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/healthz":
		w.Header().Set("cache-control", "no-store")
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	case strings.HasPrefix(path, "/desktop/project/"):
		server.files.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/desktop/components/"):
		server.components.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/desktop/system/"):
		server.system.ServeHTTP(w, r)
	case path == "/desktop/external":
		server.external.ServeHTTP(w, r)
	case path == "/desktop/quit":
		server.quit.ServeHTTP(w, r)
	case path == "/desktop/export" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/desktop/"):
		server.static.applySecureHeaders(w)
		http.Error(w, "Not Found", http.StatusNotFound)
	default:
		server.static.ServeHTTP(w, r)
	}
}
