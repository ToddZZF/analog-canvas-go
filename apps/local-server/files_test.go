package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleProjectText = `{"id":"proj-1","name":"Sample","structureRevision":0,"documents":[]}`

func renamedProjectText(name string, structureRevision int) string {
	return fmt.Sprintf(`{"id":"proj-1","name":%q,"structureRevision":%d,"documents":[]}`, name, structureRevision)
}

type testServer struct {
	server   *httptest.Server
	dataDir  string
	projects string
	client   *http.Client
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dataDir := t.TempDir()
	projects := filepath.Join(dataDir, "Projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	library := newProjectLibrary(filepath.Join(dataDir, "state"), projects)
	staticHandler, err := newStaticHandlerFromFS(testEditorBundle())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(&localServer{
		static:     staticHandler,
		files:      newFileHandler(library, filepath.Join(dataDir, "state"), nil),
		components: newComponentHandler(filepath.Join(dataDir, "components.json"), nil),
		system:     newSystemHandler(library, dataDir, "test", strings.Repeat("a", 40), false, nil),
		external:   newExternalHandler(strings.Repeat("a", 40), nil),
		quit:       newQuitHandler(nil),
	})
	t.Cleanup(server.Close)
	// The handlers validate the request Origin against the served origins.
	handlerOrigins(server)
	return &testServer{
		server:   server,
		dataDir:  dataDir,
		projects: projects,
		client:   server.Client(),
	}
}

// handlerOrigins registers the test server's own URL as an allowed write origin.
func handlerOrigins(server *httptest.Server) {
	mux, ok := server.Config.Handler.(*localServer)
	if !ok {
		panic("unexpected handler")
	}
	allow := func(current map[string]bool) map[string]bool {
		next := map[string]bool{}
		for key := range current {
			next[key] = true
		}
		next[server.URL] = true
		return next
	}
	mux.files.origins = allow(mux.files.origins)
	mux.components.origins = allow(mux.components.origins)
	mux.system.origins = allow(mux.system.origins)
	mux.external.origins = allow(mux.external.origins)
	mux.quit.origins = allow(mux.quit.origins)
}

func (test *testServer) post(t *testing.T, path string, body any) map[string]any {
	t.Helper()
	return test.postExpectingStatus(t, path, body, http.StatusOK)
}

func (test *testServer) postExpectingStatus(t *testing.T, path string, body any, status int) map[string]any {
	t.Helper()
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = encoded
	}
	request, err := http.NewRequest(http.MethodPost, test.server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("origin", test.server.URL)
	response, err := test.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("%s: status %d, want %d", path, response.StatusCode, status)
	}
	var outcome map[string]any
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return outcome
}

func (test *testServer) saveProject(t *testing.T, text string, binding map[string]any, creationKey string) map[string]any {
	t.Helper()
	body := map[string]any{"text": text, "name": "sample.icproj.json"}
	if binding != nil {
		body["binding"] = binding
	}
	if creationKey != "" {
		body["creationKey"] = creationKey
		body["intoLibrary"] = true
	}
	return test.post(t, "/desktop/project/save", body)
}

func (test *testServer) receipt(t *testing.T, outcome map[string]any) map[string]any {
	t.Helper()
	file, ok := outcome["file"].(map[string]any)
	if !ok {
		t.Fatalf("no file receipt in %v", outcome)
	}
	return file
}

func projectDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	return dirs
}

func TestSaveCreatesLibraryProjectWithHistory(t *testing.T) {
	test := newTestServer(t)
	first := test.saveProject(t, sampleProjectText, nil, "create-1")
	file := test.receipt(t, first)
	if file["revision"].(float64) != 1 {
		t.Fatalf("first save revision = %v", file["revision"])
	}
	path := file["path"].(string)
	if onDisk, err := os.ReadFile(path); err != nil || string(onDisk) != sampleProjectText {
		t.Fatalf("saved bytes differ: %v", err)
	}

	second := test.saveProject(t, renamedProjectText("Second", 1), file, "")
	updated := test.receipt(t, second)
	if updated["revision"].(float64) != 2 {
		t.Fatalf("second save revision = %v", updated["revision"])
	}
	if updated["id"] != file["id"] {
		t.Fatalf("save rebound the project: %v vs %v", updated["id"], file["id"])
	}

	history := test.post(t, "/desktop/project/history", map[string]any{"id": file["id"]})
	versions, ok := history["versions"].([]any)
	if !ok || len(versions) != 1 {
		t.Fatalf("history versions = %v", history["versions"])
	}
	version := versions[0].(map[string]any)
	if version["text"] != sampleProjectText {
		t.Fatalf("history kept %v", version["text"])
	}
	if history["currentText"] != renamedProjectText("Second", 1) {
		t.Fatalf("current text = %v", history["currentText"])
	}
}

func TestSaveRejectsStaleBindingAndExternalChange(t *testing.T) {
	test := newTestServer(t)
	outcome := test.saveProject(t, sampleProjectText, nil, "create-1")
	file := test.receipt(t, outcome)
	path := file["path"].(string)

	stale := test.post(t, "/desktop/project/save", map[string]any{
		"text": sampleProjectText, "name": "sample.icproj.json",
		"binding": map[string]any{"id": file["id"], "revision": 99},
	})
	if stale["status"] != "conflict" {
		t.Fatalf("stale revision outcome = %v", stale)
	}

	// The binding is current, but the file changed outside the editor.
	if err := os.WriteFile(path, []byte(`{"id":"proj-1","name":"External","structureRevision":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	external := test.saveProject(t, renamedProjectText("Second", 1), file, "")
	if external["status"] != "conflict" {
		t.Fatalf("external change outcome = %v", external)
	}
	onDisk, _ := os.ReadFile(path)
	if !strings.Contains(string(onDisk), "External") {
		t.Fatalf("external bytes were overwritten: %s", onDisk)
	}
}

func TestCreationKeyReplaysIdempotently(t *testing.T) {
	test := newTestServer(t)
	first := test.saveProject(t, sampleProjectText, nil, "retry-key")
	replay := test.saveProject(t, sampleProjectText, nil, "retry-key")
	if test.receipt(t, first)["path"] != test.receipt(t, replay)["path"] {
		t.Fatalf("replay allocated a new destination: %v vs %v",
			test.receipt(t, first)["path"], test.receipt(t, replay)["path"])
	}
	if len(projectDirs(t, test.projects)) != 1 {
		t.Fatalf("replay created extra directories")
	}
}

func TestLibraryActionsRenameDuplicateFavoriteRecyclePurge(t *testing.T) {
	test := newTestServer(t)
	outcome := test.saveProject(t, sampleProjectText, nil, "create-1")
	entry := test.receipt(t, outcome)
	// An open tab blocks library actions; close it first, like the UI does.
	test.post(t, "/desktop/project/release", map[string]any{"id": entry["id"]})
	libraryID := test.post(t, "/desktop/project/library", map[string]any{})
	projectID := libraryID["projects"].([]any)[0].(map[string]any)["id"].(string)

	renamed := test.post(t, "/desktop/project/library-action", map[string]any{
		"id": projectID, "action": "rename", "name": "Renamed",
	})
	if renamed["status"] != "done" {
		t.Fatalf("rename outcome = %v", renamed)
	}
	onDisk, _ := os.ReadFile(entry["path"].(string))
	document, _ := parseProjectDocument(onDisk)
	if document.name != "Renamed" || document.structureRevision == nil || *document.structureRevision != 1 {
		t.Fatalf("rename did not apply the transaction semantics: %s", onDisk)
	}

	test.post(t, "/desktop/project/library-action", map[string]any{
		"id": projectID, "action": "duplicate", "creationKey": "dup-1",
	})
	listing := test.post(t, "/desktop/project/library", map[string]any{})
	if len(listing["projects"].([]any)) != 2 {
		t.Fatalf("duplicate did not create a project: %v", listing["projects"])
	}
	var copyName string
	for _, item := range listing["projects"].([]any) {
		if project := item.(map[string]any); project["name"] == "Renamed (copy)" {
			copyName = "found"
		}
	}
	if copyName == "" {
		t.Fatalf("copy missing a suffixed name: %v", listing["projects"])
	}

	favorite := test.post(t, "/desktop/project/library-action", map[string]any{
		"id": projectID, "action": "favorite", "favorite": true,
	})
	if favorite["status"] != "done" {
		t.Fatalf("favorite outcome = %v", favorite)
	}
	listing = test.post(t, "/desktop/project/library", map[string]any{})
	var favoriteFlag bool
	for _, item := range listing["projects"].([]any) {
		if project := item.(map[string]any); project["id"] == projectID {
			favoriteFlag = project["favorite"].(bool)
		}
	}
	if !favoriteFlag {
		t.Fatalf("favorite not recorded: %v", listing)
	}

	// Recycling changes the entry id (a "recycle:" segment joins it), so
	// the recycled project is found by name, exactly as the renderer does.
	recycledID := func() string {
		listing := test.post(t, "/desktop/project/library", map[string]any{})
		for _, item := range listing["projects"].([]any) {
			project := item.(map[string]any)
			if project["name"] == "Renamed" {
				return project["id"].(string)
			}
		}
		t.Fatalf("project missing from listing: %v", listing["projects"])
		return ""
	}
	test.post(t, "/desktop/project/library-action", map[string]any{"id": projectID, "action": "delete"})
	listing = test.post(t, "/desktop/project/library", map[string]any{})
	recycled := false
	for _, item := range listing["projects"].([]any) {
		if project := item.(map[string]any); project["name"] == "Renamed" {
			recycled = project["recycled"].(bool)
		}
	}
	if !recycled {
		t.Fatalf("delete did not recycle the project: %v", listing["projects"])
	}
	recycledProjectID := recycledID()
	test.post(t, "/desktop/project/library-action", map[string]any{"id": recycledProjectID, "action": "restore"})
	test.post(t, "/desktop/project/library-action", map[string]any{"id": projectID, "action": "delete"})
	test.post(t, "/desktop/project/library-action", map[string]any{"id": recycledID(), "action": "purge"})
	listing = test.post(t, "/desktop/project/library", map[string]any{})
	if len(listing["projects"].([]any)) != 1 {
		t.Fatalf("purge did not remove the project: %v", listing["projects"])
	}
}

func TestVersionRestoreAndBranch(t *testing.T) {
	test := newTestServer(t)
	outcome := test.saveProject(t, sampleProjectText, nil, "create-1")
	file := test.receipt(t, outcome)
	test.saveProject(t, renamedProjectText("Second", 1), file, "")
	test.post(t, "/desktop/project/release", map[string]any{"id": file["id"]})
	// The renderer opens history from the library dialog, addressing the
	// library id rather than the closed tab's grant id.
	listing := test.post(t, "/desktop/project/library", map[string]any{})
	projectID := listing["projects"].([]any)[0].(map[string]any)["id"].(string)
	history := test.post(t, "/desktop/project/history", map[string]any{"id": projectID})
	version := history["versions"].([]any)[0].(map[string]any)

	bad := test.post(t, "/desktop/project/library-action", map[string]any{
		"id": projectID, "action": "restore-version", "version": version["id"],
		"expectedText": renamedProjectText("Wrong", 7),
	})
	if bad["status"] != "conflict" {
		t.Fatalf("stale expectedText outcome = %v", bad)
	}
	restored := test.post(t, "/desktop/project/library-action", map[string]any{
		"id": projectID, "action": "restore-version", "version": version["id"],
		"expectedText": renamedProjectText("Second", 1),
	})
	if restored["status"] != "done" {
		t.Fatalf("restore outcome = %v", restored)
	}
	onDisk, _ := os.ReadFile(file["path"].(string))
	reinstated, err := parseProjectDocument(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	if reinstated.id != "proj-1" || reinstated.name != "Sample" {
		t.Fatalf("restore did not reinstate the version: %s", onDisk)
	}

	branched := test.post(t, "/desktop/project/library-action", map[string]any{
		"id": projectID, "action": "branch-version", "version": version["id"], "creationKey": "branch-1",
	})
	listing = test.post(t, "/desktop/project/library", map[string]any{})
	projects, ok := listing["projects"].([]any)
	if !ok {
		t.Fatalf("branch response damaged the listing: %v / %v", branched, listing)
	}
	if len(projects) != 2 {
		t.Fatalf("branch did not create a project: %v", projects)
	}
}

func TestContentOpenImportsAnIndependentCopy(t *testing.T) {
	test := newTestServer(t)
	request, err := http.NewRequest(http.MethodPost, test.server.URL+"/desktop/project/open",
		strings.NewReader(fmt.Sprintf(`{"text":%q,"name":"imported.icproj.json","creationKey":"import-1"}`, sampleProjectText)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("origin", test.server.URL)
	response, err := test.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var outcome map[string]any
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome["status"] != "opened" {
		t.Fatalf("content open outcome = %v", outcome)
	}
	file := outcome["file"].(map[string]any)
	if file["revision"].(float64) != 1 {
		t.Fatalf("content open revision = %v", file["revision"])
	}
	text := outcome["text"].(string)
	document, err := parseProjectDocument([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if document.id == "proj-1" {
		t.Fatalf("import did not create an independent identity: %s", text)
	}
	if document.name != "Sample" {
		t.Fatalf("import renamed the project: %s", text)
	}
}

func TestOpenRequiresKnownRecentOrLibraryID(t *testing.T) {
	test := newTestServer(t)
	// Without a recent id and without a native dialog the open is cancelled.
	cancelled := test.post(t, "/desktop/project/open", nil)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("dialog-less open outcome = %v", cancelled)
	}
	request, _ := http.NewRequest(http.MethodPost, test.server.URL+"/desktop/project/open", nil)
	request.Header.Set("origin", test.server.URL)
	request.Header.Set("x-recent-project", "nope")
	response, err := test.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown recent status = %d", response.StatusCode)
	}
}

func TestResumeRejectsUnauthorizedOrChangedFiles(t *testing.T) {
	test := newTestServer(t)
	outcome := test.saveProject(t, sampleProjectText, nil, "create-1")
	file := test.receipt(t, outcome)
	path := file["path"].(string)
	digest := file["byteDigest"].(string)

	resumed := test.post(t, "/desktop/project/resume", map[string]any{"path": path, "byteDigest": digest})
	if resumed["status"] != "opened" {
		t.Fatalf("resume outcome = %v", resumed)
	}

	// A path the server never granted must stay unauthorized across restarts.
	other := filepath.Join(t.TempDir(), "elsewhere.icproj.json")
	if err := os.WriteFile(other, []byte(sampleProjectText), 0o600); err != nil {
		t.Fatal(err)
	}
	unauthorized := test.post(t, "/desktop/project/resume", map[string]any{"path": other, "byteDigest": digest})
	if unauthorized["status"] != "conflict" {
		t.Fatalf("unauthorized resume outcome = %v", unauthorized)
	}

	if err := os.WriteFile(path, []byte(renamedProjectText("Changed", 9)), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := test.post(t, "/desktop/project/resume", map[string]any{"path": path, "byteDigest": digest})
	if changed["status"] != "conflict" {
		t.Fatalf("changed resume outcome = %v", changed)
	}
}

func TestWriteOriginIsRequired(t *testing.T) {
	test := newTestServer(t)
	request, _ := http.NewRequest(http.MethodPost, test.server.URL+"/desktop/project/save",
		strings.NewReader(`{"text":"x","name":"x.icproj.json"}`))
	request.Header.Set("origin", "https://evil.example")
	response, err := test.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong origin status = %d", response.StatusCode)
	}
}

func TestSavedCopyAndCopyToLibrary(t *testing.T) {
	test := newTestServer(t)
	test.saveProject(t, sampleProjectText, nil, "create-1")
	recentID := test.post(t, "/desktop/project/library", map[string]any{})["recentProjects"].([]any)[0].(map[string]any)["id"].(string)
	saved := test.post(t, "/desktop/project/saved-copy", map[string]any{"id": recentID})
	if saved["status"] != "read" || saved["name"] != "Sample" {
		t.Fatalf("saved-copy outcome = %v", saved)
	}
	copied := test.post(t, "/desktop/project/copy-to-library", map[string]any{
		"id": recentID, "creationKey": "copy-2",
	})
	if copied["status"] != "done" {
		t.Fatalf("copy-to-library outcome = %v", copied)
	}
	listing := test.post(t, "/desktop/project/library", map[string]any{})
	if len(listing["projects"].([]any)) != 2 {
		t.Fatalf("copy-to-library did not add a project: %v", listing["projects"])
	}
}
