package main

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

// testFiles reaches past the HTTP surface for the tray-side calls that a
// desktop process makes directly on the file handler.
func testFiles(t *testing.T, test *testServer) *fileHandler {
	t.Helper()
	mux, ok := test.server.Config.Handler.(*localServer)
	if !ok {
		panic("unexpected handler")
	}
	return mux.files
}

func TestRecentSnapshotResolvesNamesAndSkipsMissing(t *testing.T) {
	test := newTestServer(t)
	files := testFiles(t, test)
	test.receipt(t, test.saveProject(t, renamedProjectText("Alpha", 0), nil, "recent-a"))
	second := test.receipt(t, test.saveProject(t, renamedProjectText("Beta", 1), nil, "recent-b"))

	projects := files.recentProjects(5)
	if len(projects) != 2 {
		t.Fatalf("recent snapshot = %v", projects)
	}
	// The display name comes from the project document, not the remembered
	// file name, which is project.icproj.json for library projects.
	if projects[0].Name != "Beta" || projects[1].Name != "Alpha" {
		t.Fatalf("recent names or order = %v", projects)
	}
	if projects[0].ID == second["id"] {
		t.Fatalf("recent id reused the tab grant id")
	}

	if err := os.Remove(second["path"].(string)); err != nil {
		t.Fatal(err)
	}
	projects = files.recentProjects(5)
	if len(projects) != 1 || projects[0].Name != "Alpha" {
		t.Fatalf("missing file not skipped: %v", projects)
	}

	limit := files.recentProjects(1)
	if len(limit) != 1 || limit[0].Name != "Alpha" {
		t.Fatalf("recent limit = %v", limit)
	}
}

func TestNextLaunchServesQueuedRecentAndSkipsForgotten(t *testing.T) {
	test := newTestServer(t)
	files := testFiles(t, test)
	test.saveProject(t, sampleProjectText, nil, "launch-1")
	recentID := files.recentProjects(1)[0].ID

	files.queueLaunch("forgotten")
	files.queueLaunch(recentID)
	outcome := test.post(t, "/desktop/project/next-launch", nil)
	if outcome["status"] != "queued" || outcome["id"] != recentID {
		t.Fatalf("next-launch outcome = %v", outcome)
	}
	outcome = test.post(t, "/desktop/project/next-launch", nil)
	if outcome["status"] != "empty" {
		t.Fatalf("stale launch not skipped: %v", outcome)
	}

	// The full tray-click contract: the queued id opens through the same
	// open route the editor's launch loop drives.
	request, _ := http.NewRequest(http.MethodPost, test.server.URL+"/desktop/project/open", nil)
	request.Header.Set("origin", test.server.URL)
	request.Header.Set("x-recent-project", recentID)
	response, err := test.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var opened map[string]any
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		t.Fatal(err)
	}
	if opened["status"] != "opened" {
		t.Fatalf("queued id open outcome = %v", opened)
	}
}

func TestSaveSignalsRecentChange(t *testing.T) {
	test := newTestServer(t)
	files := testFiles(t, test)
	test.saveProject(t, sampleProjectText, nil, "signal-1")
	select {
	case <-files.changed():
	case <-time.After(2 * time.Second):
		t.Fatalf("save did not signal the recent change")
	}
}
