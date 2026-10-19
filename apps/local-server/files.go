package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// The single authority for project files behind the editor: revision grants,
// the recent list, the library, and crash-safe commits. Exactly one operation
// runs at a time, mirroring the desktop shell's busy serialization. Port of
// apps/desktop/src/project-files.ts, with a content-based open for browsers.
type fileHandler struct {
	library        *projectLibrary
	stateDirectory string
	indexPath      string
	origins        map[string]bool

	operation      sync.Mutex
	grants         map[string]*grant
	recent         []recentProject
	recentLoaded   bool
	authorizations *projectAuthorizations
	creations      *projectCreations
}

type grant struct {
	id       string
	path     string
	bytes    []byte
	revision int
}

type recentProject struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

type requestError struct {
	message string
	status  int
}

type conflictError struct {
	message string
}

func (err *requestError) Error() string  { return err.message }
func (err *conflictError) Error() string { return err.message }

func newFileHandler(library *projectLibrary, stateDirectory string, origins []string) *fileHandler {
	indexPath := filepath.Join(stateDirectory, "desktop-index.json")
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &fileHandler{
		library:        library,
		stateDirectory: stateDirectory,
		indexPath:      indexPath,
		origins:        allowed,
		grants:         map[string]*grant{},
		authorizations: newProjectAuthorizations(indexPath + ".authorized"),
		creations:      newProjectCreations(indexPath + ".creations"),
	}
}

func fileReceipt(file *grant) map[string]any {
	return map[string]any{
		"id":         file.id,
		"name":       basename(file.path),
		"path":       file.path,
		"revision":   file.revision,
		"byteDigest": sha256Hex(file.bytes),
	}
}

var workspaceDigestShape = regexp.MustCompile(`^[a-f0-9]{64}$`)

// readBounded loads at most one project, refusing symlinks to oversized data.
func readBounded(path string) ([]byte, error) {
	bytes, err := readLimited(path, maxProjectBytes)
	if err == nil {
		return bytes, nil
	}
	if errors.Is(err, errFileTooLarge) {
		return nil, errors.New("Select a Project file no larger than 16 MiB")
	}
	return nil, err
}

func existingBytes(path string) ([]byte, error) {
	bytes, err := readBounded(path)
	if isEnoent(err) {
		return nil, nil
	}
	return bytes, err
}

func sameBytes(expected []byte, current []byte) bool {
	if expected == nil {
		return current == nil
	}
	return current != nil && bytes.Equal(expected, current)
}

func createdBaseline(path string, data []byte) ([]byte, error) {
	existing, err := existingBytes(path)
	if err != nil {
		return nil, err
	}
	if existing != nil && !bytes.Equal(existing, data) {
		return nil, &conflictError{message: "Creation destination changed; no file was overwritten"}
	}
	return existing, nil
}

// commitProjectFile swaps bytes under overwrite protection with a journaled
// history entry, writing through a fsynced temporary file.
func commitProjectFile(path string, expected []byte, data []byte, library *projectLibrary, independent bool) (string, error) {
	conflict := func() error {
		return &conflictError{message: "File changed outside the editor. No file was overwritten; save a separate copy."}
	}
	existing, err := existingBytes(path)
	if err != nil {
		return "", err
	}
	if !sameBytes(expected, existing) {
		return "", conflict()
	}
	var history *projectHistory
	if library != nil {
		directory, err := library.historyDirectory(path)
		if err != nil {
			return "", err
		}
		history = newProjectHistory(directory, path)
		if err := history.prepare(expected, data, independent); err != nil {
			if expected == nil {
				library.discardFailedAllocation(path)
			}
			return "", err
		}
	}
	temporary := filepath.Join(filepath.Dir(path), ".analog-canvas-save-"+newUUID()+".tmp")
	defer os.Remove(temporary)
	if err := func() error {
		file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	}(); err != nil {
		if expected == nil && library != nil {
			library.discardFailedAllocation(path)
		}
		return "", err
	}
	existing, err = existingBytes(path)
	if err != nil {
		if expected == nil && library != nil {
			library.discardFailedAllocation(path)
		}
		return "", err
	}
	if !sameBytes(expected, existing) {
		if expected == nil && library != nil {
			library.discardFailedAllocation(path)
		}
		return "", conflict()
	}
	if err := os.Rename(temporary, path); err != nil {
		if expected == nil && library != nil {
			library.discardFailedAllocation(path)
		}
		return "", err
	}
	if history != nil {
		if err := history.finish(); err != nil {
			return "The file is saved. History finalization will be retried on the next operation.", nil
		}
	}
	return "", nil
}

func (handler *fileHandler) loadRecentOnce() {
	if handler.recentLoaded {
		return
	}
	handler.recentLoaded = true
	bytes, err := readLimited(handler.indexPath, 1024*1024)
	if err != nil {
		return // A damaged index never grants renderer-provided paths.
	}
	var value []recentProject
	if json.Unmarshal(bytes, &value) != nil {
		return
	}
	kept := value[:0]
	for _, entry := range value {
		if entry.ID != "" && entry.Path != "" && entry.Name != "" {
			kept = append(kept, entry)
		}
	}
	if len(kept) > 20 {
		kept = kept[:20]
	}
	handler.recent = kept
}

func (handler *fileHandler) persistRecent() error {
	temporary := handler.indexPath + ".tmp"
	if err := writeDurable(temporary, mustJSON(handler.recent)); err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, handler.indexPath)
}

// remember records workspace authorization and the recent shortcut. The file
// write already succeeded, so index failures only degrade to warnings.
func (handler *fileHandler) remember(path string) string {
	warning := ""
	if err := handler.authorizations.remember(path); err != nil {
		warning = "The file is saved, but its workspace authorization could not be retained. Reopen it explicitly after restarting."
	}
	identifier := newUUID()
	kept := make([]recentProject, 0, len(handler.recent)+1)
	for _, entry := range handler.recent {
		if pathKey(entry.Path) == pathKey(path) {
			identifier = entry.ID
			continue
		}
		kept = append(kept, entry)
	}
	handler.recent = append([]recentProject{{ID: identifier, Path: path, Name: basename(path)}}, kept...)
	if len(handler.recent) > 20 {
		handler.recent = handler.recent[:20]
	}
	if err := handler.persistRecent(); err != nil {
		return "The file is saved, but the recent-project list could not be updated."
	}
	return warning
}

func creationKey(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok || text == "" || len(text) > 200 {
		return "", errors.New("Invalid creation identity")
	}
	return text, nil
}

func copyIdentity(creation string) string {
	if creation == "" {
		return newUUID()
	}
	return "copy-" + sha256Hex([]byte(creation))[:32]
}

// createLibraryCopy writes an independent project with its own identity and
// history, replaying a creation receipt when the response was lost.
func (handler *fileHandler) createLibraryCopy(document *projectDocument, creation string, suffix string) error {
	copyDocument, err := parseProjectDocument(mustJSON(document.raw))
	if err != nil {
		return err
	}
	copyDocument.makeIndependent(copyIdentity(creation), suffixName(document.name, suffix))
	data, err := copyDocument.serialize()
	if err != nil {
		return err
	}
	var prior *creationResolution
	replace := false
	if creation != "" {
		resolution, err := handler.creations.lookup(creation, data)
		if err != nil {
			return err
		}
		if resolution != nil {
			if resolution.newDestination {
				replace = true
			} else {
				prior = resolution
			}
		}
	}
	var destination string
	if prior != nil {
		destination = prior.path
	} else {
		var seed *creationSeed
		if creation != "" {
			key := creation
			if replace {
				key = newUUID()
			}
			seed = &creationSeed{Key: key, Bytes: data}
		}
		destination, err = handler.library.allocate(seed)
		if err != nil {
			return err
		}
	}
	var expected []byte
	if prior != nil {
		expected = prior.expected
	} else {
		if expected, err = createdBaseline(destination, data); err != nil {
			return err
		}
	}
	if creation != "" {
		if err := handler.creations.reserve(creation, destination, expected, data, replace); err != nil {
			return err
		}
	}
	_, err = commitProjectFile(destination, expected, data, handler.library, false)
	return err
}

// historyTarget resolves a project by open tab, recent shortcut or library id.
func (handler *fileHandler) historyTarget(id string) (*libraryProject, error) {
	if file, ok := handler.grants[id]; ok {
		resolved, err := realpath(file.path)
		if err != nil || pathKey(resolved) != pathKey(file.path) {
			return nil, &conflictError{message: "File moved; open it explicitly"}
		}
		return &libraryProject{ID: id, Name: basename(file.path), Path: file.path}, nil
	}
	for _, entry := range handler.recent {
		if entry.ID == id {
			resolved, err := realpath(entry.Path)
			if err != nil || pathKey(resolved) != pathKey(entry.Path) {
				return nil, &conflictError{message: "File moved; open it explicitly"}
			}
			return &libraryProject{ID: id, Name: basename(entry.Path), Path: entry.Path}, nil
		}
	}
	return handler.library.inspect(id)
}

func readRequest(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxProjectBytes*2+1))
	if err != nil {
		return &requestError{message: "Missing Project", status: http.StatusBadRequest}
	}
	if len(body) == 0 {
		return &requestError{message: "Missing Project", status: http.StatusBadRequest}
	}
	if len(body) > maxProjectBytes*2 {
		return &requestError{message: "Project exceeds request limit", status: http.StatusRequestEntityTooLarge}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return &requestError{message: "Missing Project", status: http.StatusBadRequest}
	}
	return nil
}

var projectNameShape = regexp.MustCompile(`[\x{0}-\x{1f}<>:"/\\|?*]`)

func validProjectName(name string) bool {
	if name == "" || len([]rune(name)) > 200 || basename(name) != name {
		return false
	}
	return !projectNameShape.MatchString(name)
}

func (handler *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "POST required"})
		return
	}
	w.Header().Set("cache-control", "no-store")
	if !handler.origins[r.Header.Get("Origin")] {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "Wrong origin"})
		return
	}
	if !handler.operation.TryLock() {
		writeJSON(w, http.StatusOK, map[string]any{"status": "failed", "message": "Another file operation is in progress"})
		return
	}
	defer handler.operation.Unlock()
	handler.loadRecentOnce()

	route := strings.TrimPrefix(r.URL.Path, "/desktop/project/")
	var outcome fileOutcome
	switch route {
	case "next-launch":
		outcome = map[string]any{"status": "empty"}
	case "copy-to-library", "saved-copy":
		outcome = handler.copyToLibrary(r, route)
	case "resume":
		outcome = handler.resume(r)
	case "library-action":
		outcome = handler.libraryAction(r)
	case "history":
		outcome = handler.history(r)
	case "library":
		outcome = handler.libraryListing(r)
	case "library-directory":
		outcome = map[string]any{"status": "cancelled"}
	case "reveal":
		outcome = handler.reveal(r)
	case "recent":
		files := make([]recentProject, len(handler.recent))
		copy(files, handler.recent)
		outcome = map[string]any{"status": "listed", "files": files}
	case "forget", "release":
		outcome = handler.forgetOrRelease(r, route)
	case "open":
		outcome = handler.open(r)
	case "save":
		outcome = handler.save(r)
	default:
		outcome = map[string]any{"message": "Not found"}
	}
	status := http.StatusOK
	if explicit, ok := outcome["_status"].(int); ok {
		status = explicit
		delete(outcome, "_status")
	}
	writeJSON(w, status, outcome)
}

type fileOutcome = map[string]any

func failureOutcome(err error) fileOutcome {
	var conflict *conflictError
	if errors.As(err, &conflict) {
		return map[string]any{"status": "conflict", "message": conflict.message}
	}
	var request *requestError
	if errors.As(err, &request) {
		return map[string]any{"_status": request.status, "message": request.message}
	}
	message := "File operation failed"
	if err != nil {
		message = err.Error()
	}
	return map[string]any{"status": "failed", "message": message}
}

func (handler *fileHandler) copyToLibrary(r *http.Request, route string) fileOutcome {
	var body struct {
		ID          string `json:"id"`
		CreationKey any    `json:"creationKey"`
	}
	if err := readRequest(r, &body); err != nil {
		return failureOutcome(err)
	}
	var creation string
	var err error
	if creation, err = creationKey(body.CreationKey); err != nil {
		return failureOutcome(err)
	}
	path := ""
	for _, entry := range handler.recent {
		if entry.ID == body.ID {
			path = entry.Path
			break
		}
	}
	if path == "" {
		if path, err = handler.library.resolve(body.ID); err != nil {
			return failureOutcome(err)
		}
	}
	resolved, err := realpath(path)
	if err != nil || pathKey(resolved) != pathKey(path) {
		return failureOutcome(errors.New("File moved; open it explicitly"))
	}
	bytes, err := readBounded(path)
	if err != nil {
		return failureOutcome(err)
	}
	document, err := parseProjectDocument(bytes)
	if err != nil {
		return failureOutcome(err)
	}
	if route == "saved-copy" {
		serialized, err := document.serialize()
		if err != nil {
			return failureOutcome(err)
		}
		return map[string]any{"status": "read", "name": document.name, "text": string(serialized)}
	}
	if err := handler.createLibraryCopy(document, creation, "copy"); err != nil {
		return failureOutcome(err)
	}
	return map[string]any{"status": "done"}
}

func (handler *fileHandler) resume(r *http.Request) fileOutcome {
	var body struct {
		Path       string `json:"path"`
		ByteDigest string `json:"byteDigest"`
	}
	if err := readRequest(r, &body); err != nil {
		return failureOutcome(err)
	}
	if body.Path == "" || !workspaceDigestShape.MatchString(body.ByteDigest) {
		return failureOutcome(errors.New("Invalid workspace file hint"))
	}
	if !handler.authorizations.allows(body.Path) {
		return failureOutcome(&conflictError{message: "This file is no longer remembered. Open it explicitly or save the recovered copy separately."})
	}
	resolved, err := realpath(body.Path)
	if err != nil {
		return failureOutcome(&conflictError{message: "The remembered file moved; open it explicitly"})
	}
	if pathKey(resolved) != pathKey(body.Path) {
		return failureOutcome(&conflictError{message: "The remembered file moved; open it explicitly"})
	}
	current, err := readBounded(resolved)
	if err != nil {
		return failureOutcome(err)
	}
	if sha256Hex(current) != body.ByteDigest {
		return failureOutcome(&conflictError{message: "The file changed while the app was closed; the recovered Project remains unbound"})
	}
	var existing *grant
	for _, candidate := range handler.grants {
		if pathKey(candidate.path) == pathKey(resolved) {
			existing = candidate
			break
		}
	}
	if existing != nil && !bytes.Equal(existing.bytes, current) {
		return failureOutcome(&conflictError{message: "An open Project already holds an earlier version of this file"})
	}
	file := existing
	if file == nil {
		file = &grant{id: newUUID(), path: resolved, bytes: current, revision: 1}
	}
	handler.grants[file.id] = file
	return map[string]any{"status": "opened", "file": fileReceipt(file), "text": string(file.bytes)}
}

func (handler *fileHandler) libraryAction(r *http.Request) fileOutcome {
	var body struct {
		ID           string `json:"id"`
		Action       string `json:"action"`
		Name         string `json:"name"`
		Favorite     *bool  `json:"favorite"`
		Version      string `json:"version"`
		ExpectedText string `json:"expectedText"`
		CreationKey  any    `json:"creationKey"`
	}
	if err := readRequest(r, &body); err != nil {
		return failureOutcome(err)
	}
	if body.ID == "" {
		return map[string]any{"_status": http.StatusBadRequest, "message": "Invalid Project id"}
	}
	creation, err := creationKey(body.CreationKey)
	if err != nil {
		return failureOutcome(err)
	}
	var entry *libraryProject
	if body.Action == "restore-version" || body.Action == "branch-version" {
		if entry, err = handler.historyTarget(body.ID); err != nil {
			return failureOutcome(err)
		}
	} else {
		if entry, err = handler.library.inspect(body.ID); err != nil {
			return failureOutcome(err)
		}
	}
	if contains([]string{"rename", "delete", "purge", "restore", "restore-version"}, body.Action) {
		for _, file := range handler.grants {
			if pathKey(file.path) == pathKey(entry.Path) {
				return map[string]any{"status": "conflict", "message": "Close this Project's tab before changing its saved copy. Save or discard edits using the tab's close prompt."}
			}
		}
	}
	switch {
	case body.Action == "favorite" && body.Favorite != nil:
		if err := handler.library.favorite(body.ID, *body.Favorite); err != nil {
			return failureOutcome(err)
		}
	case body.Action == "restore-version" || body.Action == "branch-version":
		if entry.Recycled {
			return failureOutcome(errors.New("Restore this Project before changing it"))
		}
		directory, err := handler.library.historyDirectory(entry.Path)
		if err != nil {
			return failureOutcome(err)
		}
		versions, err := newProjectHistory(directory, entry.Path).list()
		if err != nil {
			return failureOutcome(err)
		}
		var version *listedVersion
		for index := range versions {
			if versions[index].ID == body.Version {
				version = &versions[index]
				break
			}
		}
		if version == nil {
			return failureOutcome(errors.New("This historical version is no longer available; refresh history"))
		}
		document, err := parseProjectDocument([]byte(version.Text))
		if err != nil {
			return failureOutcome(err)
		}
		if body.Action == "branch-version" {
			if err := handler.createLibraryCopy(document, creation, "branch"); err != nil {
				return failureOutcome(err)
			}
		} else {
			if body.ExpectedText == "" {
				return failureOutcome(errors.New("Refresh history before restoring"))
			}
			expected := []byte(body.ExpectedText)
			if current, err := existingBytes(entry.Path); err != nil {
				return failureOutcome(err)
			} else if !sameBytes(expected, current) {
				return failureOutcome(&conflictError{message: "Project changed since history was opened; refresh before restoring"})
			}
			current, err := parseProjectDocument(expected)
			if err != nil {
				return failureOutcome(err)
			}
			if document.id != current.id {
				return failureOutcome(errors.New("Historical version belongs to another Project; create a branch instead"))
			}
			// The desktop confirms natively; the browser already confirmed
			// in the page, and the current version stays in history.
			serialized, err := document.serialize()
			if err != nil {
				return failureOutcome(err)
			}
			if _, err := commitProjectFile(entry.Path, expected, serialized, handler.library, false); err != nil {
				return failureOutcome(err)
			}
		}
	case body.Action == "delete" || body.Action == "purge":
		if body.Action == "delete" {
			if err := handler.library.recycle(body.ID, false); err != nil {
				return failureOutcome(err)
			}
		} else {
			if err := handler.library.purge(body.ID); err != nil {
				return failureOutcome(err)
			}
		}
		kept := handler.recent[:0]
		for _, recentEntry := range handler.recent {
			if pathKey(recentEntry.Path) != pathKey(entry.Path) {
				kept = append(kept, recentEntry)
			}
		}
		handler.recent = kept
		if err := handler.persistRecent(); err != nil {
			return failureOutcome(err)
		}
	case body.Action == "restore":
		if err := handler.library.recycle(body.ID, true); err != nil {
			return failureOutcome(err)
		}
	case body.Action == "rename" || body.Action == "duplicate":
		if entry.Recycled {
			return failureOutcome(errors.New("Restore this Project before changing it"))
		}
		before, err := readBounded(entry.Path)
		if err != nil {
			return failureOutcome(err)
		}
		document, err := parseProjectDocument(before)
		if err != nil {
			return failureOutcome(err)
		}
		if body.Action == "rename" {
			name := strings.TrimSpace(body.Name)
			if name == "" || len(name) > 120 {
				return failureOutcome(errors.New("Use a name between 1 and 120 characters"))
			}
			document.setName(name)
			serialized, err := document.serialize()
			if err != nil {
				return failureOutcome(err)
			}
			if _, err := commitProjectFile(entry.Path, before, serialized, handler.library, false); err != nil {
				return failureOutcome(err)
			}
		} else {
			if err := handler.createLibraryCopy(document, creation, "copy"); err != nil {
				return failureOutcome(err)
			}
		}
	default:
		return map[string]any{"_status": http.StatusBadRequest, "message": "Unknown library action"}
	}
	return map[string]any{"status": "done"}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (handler *fileHandler) history(r *http.Request) fileOutcome {
	var body struct {
		ID string `json:"id"`
	}
	if err := readRequest(r, &body); err != nil {
		return failureOutcome(err)
	}
	if body.ID == "" {
		return map[string]any{"_status": http.StatusBadRequest, "message": "Invalid Project id"}
	}
	target, err := handler.historyTarget(body.ID)
	if err != nil {
		return failureOutcome(err)
	}
	directory, err := handler.library.historyDirectory(target.Path)
	if err != nil {
		return failureOutcome(err)
	}
	versions, err := newProjectHistory(directory, target.Path).list()
	if err != nil {
		return failureOutcome(err)
	}
	current, err := readBounded(target.Path)
	if err != nil {
		return failureOutcome(err)
	}
	return map[string]any{"status": "listed", "versions": versions, "currentText": string(current)}
}

func (handler *fileHandler) libraryListing(r *http.Request) fileOutcome {
	listing, err := handler.library.list()
	if err != nil {
		return map[string]any{"_status": http.StatusNotFound, "message": "Project library unavailable"}
	}
	recentProjects := make([]map[string]any, 0, len(handler.recent))
	for _, entry := range handler.recent {
		item := map[string]any{"id": entry.ID, "name": entry.Name, "path": entry.Path}
		resolved, err := realpath(entry.Path)
		if err != nil || pathKey(resolved) != pathKey(entry.Path) {
			item["error"] = "File moved; open it explicitly"
		} else if documentBytes, err := readBounded(entry.Path); err != nil {
			item["error"] = "File unavailable"
		} else if document, err := parseProjectDocument(documentBytes); err != nil {
			item["error"] = "File unavailable"
		} else {
			item["name"] = document.name
		}
		recentProjects = append(recentProjects, item)
	}
	return map[string]any{
		"status":         "listed",
		"root":           listing.Root,
		"roots":          listing.Roots,
		"projects":       listing.Projects,
		"recentProjects": recentProjects,
	}
}

func (handler *fileHandler) reveal(r *http.Request) fileOutcome {
	var body struct {
		ID string `json:"id"`
	}
	_ = readRequest(r, &body)
	path := ""
	if body.ID != "" {
		for _, entry := range handler.recent {
			if entry.ID == body.ID {
				path = entry.Path
				break
			}
		}
		if path == "" {
			if file, ok := handler.grants[body.ID]; ok {
				path = file.path
			}
		}
		if path == "" {
			var err error
			if path, err = handler.library.locate(body.ID); err != nil {
				return failureOutcome(err)
			}
		}
	} else {
		listing, err := handler.library.list()
		if err != nil {
			return map[string]any{"_status": http.StatusNotFound, "message": "Project library unavailable"}
		}
		path = listing.Root
	}
	if err := openInFileManager(path); err != nil {
		return failureOutcome(err)
	}
	return map[string]any{"status": "done"}
}

func (handler *fileHandler) forgetOrRelease(r *http.Request, route string) fileOutcome {
	var body struct {
		ID string `json:"id"`
	}
	if err := readRequest(r, &body); err != nil {
		return map[string]any{"_status": http.StatusBadRequest, "message": "Invalid id"}
	}
	if route == "release" {
		delete(handler.grants, body.ID)
	} else {
		kept := handler.recent[:0]
		for _, entry := range handler.recent {
			if entry.ID != body.ID {
				kept = append(kept, entry)
			}
		}
		handler.recent = kept
		if err := handler.persistRecent(); err != nil {
			return failureOutcome(err)
		}
	}
	return map[string]any{"status": "done"}
}

// open serves remembered and library projects. A body carrying project text
// imports that content as a new independent library project, which is how the
// browser opens a file chosen through its own file picker.
func (handler *fileHandler) open(r *http.Request) fileOutcome {
	requestedID := r.Header.Get("x-recent-project")
	var body struct {
		Text        string `json:"text"`
		Name        string `json:"name"`
		CreationKey any    `json:"creationKey"`
	}
	bodyBytes, readErr := io.ReadAll(io.LimitReader(r.Body, maxProjectBytes*2+1))
	hasBody := readErr == nil && len(bodyBytes) > 0 && len(bodyBytes) <= maxProjectBytes*2
	if readErr != nil || len(bodyBytes) > maxProjectBytes*2 {
		return failureOutcome(&requestError{message: "Project exceeds request limit", status: http.StatusRequestEntityTooLarge})
	}
	if hasBody && json.Unmarshal(bodyBytes, &body) == nil && body.Text != "" {
		return handler.openContent(body)
	}
	var remembered *recentProject
	if requestedID != "" {
		for index := range handler.recent {
			if handler.recent[index].ID == requestedID {
				remembered = &handler.recent[index]
				break
			}
		}
	}
	libraryPath := ""
	if strings.HasPrefix(requestedID, "library:") {
		resolved, err := handler.library.resolve(requestedID)
		if err != nil {
			return failureOutcome(err)
		}
		libraryPath = resolved
	}
	if requestedID != "" && remembered == nil && libraryPath == "" {
		return map[string]any{"_status": http.StatusNotFound, "message": "Unknown recent Project"}
	}
	chosen := libraryPath
	if chosen == "" && remembered != nil {
		chosen = remembered.Path
	}
	if chosen == "" {
		// No native dialog exists in a browser; the page opens files
		// through its own picker, which arrives as a content open.
		return map[string]any{"status": "cancelled"}
	}
	path, err := realpath(chosen)
	if err != nil {
		return failureOutcome(err)
	}
	fileBytes, err := readBounded(path)
	if err != nil {
		return failureOutcome(err)
	}
	var file *grant
	for _, candidate := range handler.grants {
		if pathKey(candidate.path) == pathKey(path) {
			file = candidate
			break
		}
	}
	if file == nil {
		file = &grant{id: newUUID(), path: path, bytes: fileBytes, revision: 1}
	}
	handler.grants[file.id] = file
	handler.remember(path)
	return map[string]any{"status": "opened", "file": fileReceipt(file), "text": string(file.bytes)}
}

func (handler *fileHandler) openContent(body struct {
	Text        string `json:"text"`
	Name        string `json:"name"`
	CreationKey any    `json:"creationKey"`
}) fileOutcome {
	data := []byte(body.Text)
	if len(data) > maxProjectBytes {
		return map[string]any{"message": "Project exceeds 16 MiB"}
	}
	document, err := parseProjectDocument(data)
	if err != nil {
		return failureOutcome(err)
	}
	name := document.name
	if name == "" {
		name = strings.TrimSuffix(body.Name, ".icproj.json")
	}
	if name == "" {
		name = "Imported Project"
	}
	creation, err := creationKey(body.CreationKey)
	if err != nil {
		return failureOutcome(err)
	}
	importDocument, err := parseProjectDocument(data)
	if err != nil {
		return failureOutcome(err)
	}
	importDocument.makeIndependent(copyIdentity(creation), name)
	serialized, err := importDocument.serialize()
	if err != nil {
		return failureOutcome(err)
	}
	var seed *creationSeed
	if creation != "" {
		seed = &creationSeed{Key: creation, Bytes: serialized}
	}
	destination, err := handler.library.allocate(seed)
	if err != nil {
		return failureOutcome(err)
	}
	expected, err := createdBaseline(destination, serialized)
	if err != nil {
		return failureOutcome(err)
	}
	if creation != "" {
		if err := handler.creations.reserve(creation, destination, expected, serialized, false); err != nil {
			return failureOutcome(err)
		}
	}
	if _, err := commitProjectFile(destination, expected, serialized, handler.library, true); err != nil {
		return failureOutcome(err)
	}
	path, err := realpath(destination)
	if err != nil {
		return failureOutcome(err)
	}
	file := &grant{id: newUUID(), path: path, bytes: serialized, revision: 1}
	handler.grants[file.id] = file
	handler.remember(path)
	return map[string]any{"status": "opened", "file": fileReceipt(file), "text": string(serialized)}
}

func (handler *fileHandler) save(r *http.Request) fileOutcome {
	var body struct {
		Text    string `json:"text"`
		Name    string `json:"name"`
		Binding *struct {
			ID       string `json:"id"`
			Revision int    `json:"revision"`
		} `json:"binding"`
		SaveAs      *bool `json:"saveAs"`
		IntoLibrary *bool `json:"intoLibrary"`
		CreationKey any   `json:"creationKey"`
	}
	if err := readRequest(r, &body); err != nil {
		return failureOutcome(err)
	}
	if body.Text == "" || !validProjectName(body.Name) {
		return map[string]any{"_status": http.StatusBadRequest, "message": "Invalid Project request"}
	}
	saveAs := body.SaveAs != nil && *body.SaveAs
	intoLibrary := body.IntoLibrary != nil && *body.IntoLibrary
	data := []byte(body.Text)
	if len(data) > maxProjectBytes {
		return map[string]any{"_status": http.StatusRequestEntityTooLarge, "message": "Project exceeds 16 MiB"}
	}
	var bound *grant
	if body.Binding != nil && body.Binding.ID != "" {
		bound = handler.grants[body.Binding.ID]
	}
	if body.Binding != nil && (bound == nil || body.Binding.Revision != bound.revision) && !saveAs {
		return map[string]any{"status": "conflict", "message": "File binding expired or changed. Use Save As with a new destination."}
	}
	choose := bound == nil || saveAs
	creation, err := creationKey(body.CreationKey)
	if err != nil {
		return failureOutcome(err)
	}
	var resolution *creationResolution
	if choose && creation != "" {
		if resolution, err = handler.creations.lookup(creation, data); err != nil {
			return failureOutcome(err)
		}
	}
	var priorCreation *creationResolution
	replaceCreation := false
	if resolution != nil {
		if resolution.newDestination {
			replaceCreation = true
		} else {
			priorCreation = resolution
		}
	}
	allocatedWithKey := false
	var path string
	if bound != nil {
		path = bound.path
	}
	if choose {
		allocateInLibrary := (body.Binding == nil && !saveAs) || (saveAs && intoLibrary)
		allocatedWithKey = allocateInLibrary && creation != ""
		var chosen string
		switch {
		case priorCreation != nil:
			chosen = priorCreation.path
		case allocateInLibrary:
			var seed *creationSeed
			if creation != "" {
				key := creation
				if replaceCreation {
					key = newUUID()
				}
				seed = &creationSeed{Key: key, Bytes: data}
			}
			if chosen, err = handler.library.allocate(seed); err != nil {
				return failureOutcome(err)
			}
		default:
			// A browser cannot host the native save dialog; Save As is
			// offered against the project library instead.
			return map[string]any{"status": "cancelled"}
		}
		if resolved, err := realpath(chosen); err == nil {
			path = resolved
		} else if isEnoent(err) {
			parent, parentErr := realpath(filepath.Dir(chosen))
			if parentErr != nil {
				return failureOutcome(parentErr)
			}
			path = filepath.Join(parent, basename(chosen))
		} else {
			return failureOutcome(err)
		}
	}
	if path == "" {
		return failureOutcome(errors.New("No authorized destination"))
	}
	if saveAs && bound != nil && pathKey(path) == pathKey(bound.path) {
		return map[string]any{"status": "conflict", "message": "Save As creates an independent Project. Choose a different file or the project library."}
	}
	var owner *grant
	for _, candidate := range handler.grants {
		if pathKey(candidate.path) == pathKey(path) {
			owner = candidate
			break
		}
	}
	bindingRevision := -1
	if body.Binding != nil {
		bindingRevision = body.Binding.Revision
	}
	replay := (allocatedWithKey || (priorCreation != nil && priorCreation.committed)) &&
		(owner == nil || bytes.Equal(owner.bytes, data))
	if owner != nil && owner != bound && !replay {
		return map[string]any{"status": "conflict", "message": "That destination belongs to another open Project. Choose a different file."}
	}
	if owner != nil && bindingRevision != owner.revision && !replay {
		return map[string]any{"status": "conflict", "message": "That file has a newer save. Choose a different destination."}
	}
	var expected []byte
	if priorCreation != nil {
		expected = priorCreation.expected
	} else if allocatedWithKey {
		if expected, err = createdBaseline(path, data); err != nil {
			return failureOutcome(err)
		}
	} else if owner != nil {
		expected = owner.bytes
	} else {
		if expected, err = existingBytes(path); err != nil {
			return failureOutcome(err)
		}
	}
	if saveAs && expected != nil && !replay && priorCreation == nil {
		// The desktop confirms natively first; archiving keeps the
		// replaced version and its history recoverable either way.
		if err := handler.library.archiveReplacement(path, expected); err != nil {
			return failureOutcome(err)
		}
	}
	if choose && creation != "" {
		if err := handler.creations.reserve(creation, path, expected, data, replaceCreation); err != nil {
			return failureOutcome(err)
		}
	}
	historyWarning, err := commitProjectFile(path, expected, data, handler.library, saveAs)
	if err != nil {
		return failureOutcome(err)
	}
	file := owner
	if file == nil {
		file = &grant{id: newUUID(), path: path, revision: 0}
	}
	file.bytes = data
	file.revision++
	handler.grants[file.id] = file
	if bound != nil && bound.id != file.id {
		delete(handler.grants, bound.id)
	}
	warning := handler.remember(path)
	if warning == "" {
		warning = historyWarning
	}
	outcome := map[string]any{"status": "saved", "file": fileReceipt(file)}
	if warning != "" {
		outcome["warning"] = warning
	}
	return outcome
}
