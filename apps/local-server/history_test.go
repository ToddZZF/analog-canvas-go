package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An interrupted save journals the transition before the rename; recovery
// must resolve it by comparing bytes, never inventing a version.
func TestHistoryPendingJournalRecovery(t *testing.T) {
	dataDir := t.TempDir()
	projectPath := filepath.Join(dataDir, "project.icproj.json")
	history := newProjectHistory(filepath.Join(dataDir, ".history"), projectPath)

	if err := os.WriteFile(projectPath, []byte("version-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Simulate a process death between journal and rename: the pending
	// record and both snapshots exist, the project file is untouched.
	if err := history.prepare([]byte("version-one"), []byte("version-two"), false); err != nil {
		t.Fatal(err)
	}

	// The rename never happened, so the save is treated as not committed
	// and the history stays exactly as it was (empty).
	versions, err := history.list()
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("uncommitted save invented versions: %+v", versions)
	}

	// The same interruption with the rename already done resolves to the
	// committed branch: the previous bytes become the retained version.
	committed := newProjectHistory(filepath.Join(dataDir, ".history-committed"), projectPath)
	if err := committed.prepare([]byte("version-one"), []byte("version-two"), false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte("version-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	versions, err = committed.list()
	if err != nil {
		t.Fatalf("recovery after commit failed: %v", err)
	}
	if len(versions) != 1 || versions[0].Text != "version-one" {
		t.Fatalf("post-commit versions = %+v", versions)
	}

	// A file matching neither journal side is a conflict; the retained
	// history is kept and nothing is overwritten.
	if err := history.prepare([]byte("version-two"), []byte("version-three"), false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte("something-else"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := history.list(); err == nil ||
		!strings.Contains(err.Error(), "interrupted save conflicts") {
		t.Fatalf("conflicting recovery error = %v", err)
	}
}

func TestListingClearsThePendingCreateMarker(t *testing.T) {
	test := newTestServer(t)
	outcome := test.saveProject(t, sampleProjectText, nil, "create-1")
	path := test.receipt(t, outcome)["path"].(string)
	marker := filepath.Join(filepath.Dir(path), ".pending-create")
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("pending marker missing after first save: %v", err)
	}
	listing := test.post(t, "/desktop/project/library", map[string]any{})
	projects := listing["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("listing = %v", projects)
	}
	entry := projects[0].(map[string]any)
	if entry["warning"] != nil {
		t.Fatalf("committed save recovered as interrupted: %v", entry)
	}
	if _, err := os.Lstat(marker); !isEnoent(err) {
		t.Fatalf("listing did not clear the pending marker: %v", err)
	}
}

// A failed first commit of a fresh allocation must leave no half-written
// project file behind; the allocation directory stays retriable.
func TestSaveIntoReadOnlyDirectoryFailsWithoutPartialFiles(t *testing.T) {
	test := newTestServer(t)
	listing := test.post(t, "/desktop/project/library", map[string]any{})
	root := listing["root"].(string)
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	outcome := test.saveProject(t, sampleProjectText, nil, "create-ro")
	if outcome["status"] != "failed" {
		t.Fatalf("read-only save outcome = %v", outcome)
	}
	if dirs := projectDirs(t, root); len(dirs) != 0 {
		t.Fatalf("failed allocation left directories behind: %v", dirs)
	}
}
