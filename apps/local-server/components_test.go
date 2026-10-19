package main

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func TestComponentSaveRevisionAndIdempotency(t *testing.T) {
	test := newTestServer(t)
	definition := map[string]any{"symbol": map[string]any{"name": "My Resistor"}}
	first := test.post(t, "/desktop/components/save", map[string]any{
		"id": "comp-1", "revision": 0, "definition": definition,
		"idempotencyKey": "save-1",
	})
	entry := first["entry"].(map[string]any)
	if entry["revision"].(float64) != 1 || entry["authorId"] != "desktop-local" {
		t.Fatalf("first save entry = %v", entry)
	}
	if _, hasReceipt := entry["receipt"]; hasReceipt {
		t.Fatalf("receipt leaked into the public entry: %v", entry)
	}

	replay := test.post(t, "/desktop/components/save", map[string]any{
		"id": "comp-1", "revision": 0, "definition": definition,
		"idempotencyKey": "save-1",
	})
	if replay["entry"].(map[string]any)["revision"].(float64) != 1 {
		t.Fatalf("idempotent replay returned %v", replay)
	}

	conflict := test.postExpectingStatus(t, "/desktop/components/save", map[string]any{
		"id": "comp-1", "revision": 0, "definition": definition,
		"idempotencyKey": "save-2",
	}, http.StatusConflict)
	if conflict["error"] == nil {
		t.Fatalf("reused key with different content accepted: %v", conflict)
	}

	stale := test.postExpectingStatus(t, "/desktop/components/save", map[string]any{
		"id": "comp-1", "revision": 0, "definition": definition,
	}, http.StatusConflict)
	if stale["error"] == nil || !strings.Contains(stale["error"].(string), "changed") {
		t.Fatalf("stale revision accepted: %v", stale)
	}

	listing := test.post(t, "/desktop/components/list", map[string]any{})
	entries := listing["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("list = %v", entries)
	}
}

func TestComponentDeleteRestoreLifecycle(t *testing.T) {
	test := newTestServer(t)
	test.post(t, "/desktop/components/save", map[string]any{
		"id": "comp-1", "revision": 0,
		"definition": map[string]any{"symbol": map[string]any{"name": "X"}},
	})
	deleted := test.post(t, "/desktop/components/delete", map[string]any{"id": "comp-1", "revision": 1})
	if deleted["entry"].(map[string]any)["status"] != "deleted" {
		t.Fatalf("delete entry = %v", deleted)
	}
	blocked := test.postExpectingStatus(t, "/desktop/components/save", map[string]any{
		"id": "comp-1", "revision": 2,
		"definition": map[string]any{"symbol": map[string]any{"name": "X"}},
	}, http.StatusConflict)
	if blocked["error"] == nil {
		t.Fatalf("editing a deleted component succeeded: %v", blocked)
	}
	restored := test.post(t, "/desktop/components/restore", map[string]any{"id": "comp-1", "revision": 2})
	if restored["entry"].(map[string]any)["status"] != "shared" {
		t.Fatalf("restore entry = %v", restored)
	}
	checked := test.post(t, "/desktop/components/check", map[string]any{"id": "comp-1", "revision": 3})
	if checked["entry"] == nil {
		t.Fatalf("check entry = %v", checked)
	}
}

func TestStaticServingContract(t *testing.T) {
	test := newTestServer(t)
	base := test.server.URL
	response, err := test.client.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("index status = %d", response.StatusCode)
	}
	if cache := response.Header.Get("cache-control"); cache != "no-cache" {
		t.Fatalf("index cache-control = %q", cache)
	}
	policy := response.Header.Get("content-security-policy")
	if !strings.Contains(policy, "default-src 'self'") || !strings.Contains(policy, "'wasm-unsafe-eval'") {
		t.Fatalf("CSP missing foundations: %s", policy)
	}
	if response.Header.Get("cross-origin-opener-policy") != "same-origin" {
		t.Fatalf("COOP missing")
	}
	if response.Header.Get("x-content-type-options") != "nosniff" {
		t.Fatalf("nosniff missing")
	}

	missing, err := test.client.Get(base + "/assets/does-not-exist.js")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", missing.StatusCode)
	}

	route, err := test.client.Get(base + "/editor")
	if err != nil {
		t.Fatal(err)
	}
	defer route.Body.Close()
	if route.StatusCode != http.StatusOK {
		t.Fatalf("editor route status = %d", route.StatusCode)
	}

	cloud, err := test.client.Get(base + "/api/gallery")
	if err != nil {
		t.Fatal(err)
	}
	defer cloud.Body.Close()
	if cloud.StatusCode != http.StatusNotFound {
		t.Fatalf("cloud API surface must stay absent, got %d", cloud.StatusCode)
	}
}

func TestInlineScriptHashesFollowTheDocument(t *testing.T) {
	document := "<html><head><script>bootstrap()</script><script src=\"app.js\"></script>" +
		"<script>  </script><script>second()</script></head></html>"
	hashes := inlineScriptHashes(document)
	if len(hashes) != 2 {
		t.Fatalf("inline hashes = %v, want exactly the two non-empty inline scripts", hashes)
	}
	first := sha256.Sum256([]byte("bootstrap()"))
	expected := "'sha256-" + base64.StdEncoding.EncodeToString(first[:]) + "'"
	if hashes[0] != expected {
		t.Fatalf("first hash = %s, want %s", hashes[0], expected)
	}
}

func TestSystemInfoExposesBrowserCapabilities(t *testing.T) {
	test := newTestServer(t)
	outcome := test.post(t, "/desktop/system/info", map[string]any{})
	info := outcome["info"].(map[string]any)
	if info["format"].(float64) != currentProjectFileVersion {
		t.Fatalf("format = %v", info["format"])
	}
	if info["packaged"] != false {
		t.Fatalf("packaged must be false so association controls stay disabled")
	}
	if info["projectRoot"] == "" || info["userData"] == "" {
		t.Fatalf("data locations missing: %v", info)
	}
}

// testEditorBundle stands in for the real pnpm build output.
func testEditorBundle() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			"<!doctype html><html><head><script>bootstrap()</script>" +
				"<script src=\"/assets/app.js\"></script></head>" +
				"<body><div id=\"root\"></div></body></html>",
		)},
		"assets/app.js": &fstest.MapFile{Data: []byte("export {};")},
	}
}
