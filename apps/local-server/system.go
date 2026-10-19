package main

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// The About panel's data plus the fixed external-link allowlist. The About
// dialog reports packaged:false, which disables the Windows file-association
// controls a browser build cannot offer. Port of system-info.ts and
// external-links.ts.
type systemHandler struct {
	library       *projectLibrary
	dataDirectory string
	version       string
	commit        string
	dirty         bool
	projectFormat int
	origins       map[string]bool
	operation     sync.Mutex
	migration     string
}

func newSystemHandler(library *projectLibrary, dataDirectory string, version string, commit string, dirty bool, origins []string) *systemHandler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &systemHandler{
		library:       library,
		dataDirectory: dataDirectory,
		version:       version,
		commit:        commit,
		dirty:         dirty,
		projectFormat: currentProjectFileVersion,
		origins:       allowed,
		migration:     "",
	}
}

const currentProjectFileVersion = 72

var commitShape = regexp.MustCompile(`^[a-f0-9]{40}$`)

func (handler *systemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "POST required"})
		return
	}
	if !handler.origins[r.Header.Get("Origin")] {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "Wrong origin"})
		return
	}
	handler.operation.Lock()
	defer handler.operation.Unlock()
	w.Header().Set("cache-control", "no-store")
	action := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	switch action {
	case "info":
		projectRoot := ""
		if listing, err := handler.library.list(); err == nil {
			projectRoot = listing.Root
		} else {
			projectRoot = handler.library.defaultRoot
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ready",
			"info": map[string]any{
				"version":     handler.version,
				"commit":      handler.commit,
				"dirty":       handler.dirty,
				"format":      handler.projectFormat,
				"userData":    handler.dataDirectory,
				"projectRoot": projectRoot,
				"packaged":    false,
				"migration":   handler.migration,
				"association": map[string]any{"enabled": false, "otherInstallation": false},
			},
		})
	case "enable", "disable":
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "failed",
			"message": "File association belongs to the packaged desktop application",
		})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Unknown settings operation"})
	}
}

var externalTargets = map[string]string{
	"owner":      "https://tokenzhang.com",
	"repository": "https://github.com/cascode-ai/analog-canvas",
	"rnl":        "https://rich-netlist.github.io/rnl/en/",
}

type externalHandler struct {
	commit  string
	origins map[string]bool
}

func newExternalHandler(commit string, origins []string) *externalHandler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &externalHandler{commit: commit, origins: allowed}
}

func (handler *externalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "POST required"})
		return
	}
	if !handler.origins[r.Header.Get("Origin")] {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "Wrong origin"})
		return
	}
	w.Header().Set("cache-control", "no-store")
	var body struct {
		Target string `json:"target"`
	}
	if err := readStrictJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Link could not open"})
		return
	}
	url, known := externalTargets[body.Target]
	if !known {
		if body.Target == "source" {
			if !commitShape.MatchString(handler.commit) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Source identity unavailable"})
				return
			}
			url = "https://github.com/cascode-ai/analog-canvas/commit/" + handler.commit
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Link could not open"})
			return
		}
	}
	if err := openInBrowser(url); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Link could not open"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "opened"})
}
