package main

import (
	"net/http"
	"sync"
)

// The editor's File → Quit Local Server, for desktops whose panel cannot
// host the tray icon. It answers first and then runs the same graceful
// shutdown as the tray Quit and Ctrl+C; Shutdown waits for this response
// to reach the page.
type quitHandler struct {
	origins map[string]bool
	once    sync.Once
	request chan struct{}
}

func newQuitHandler(origins []string) *quitHandler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &quitHandler{origins: allowed, request: make(chan struct{})}
}

func (handler *quitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "POST required"})
		return
	}
	if !handler.origins[r.Header.Get("Origin")] {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "Wrong origin"})
		return
	}
	w.Header().Set("cache-control", "no-store")
	handler.once.Do(func() { close(handler.request) })
	writeJSON(w, http.StatusOK, map[string]any{"status": "quitting"})
}
