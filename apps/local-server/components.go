package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The private, revision-checked local component library. Project captures
// never depend on this file; entries are opaque JSON the renderer validates
// with the shared contract parser on read. Port of apps/desktop/src/component-library.ts.
type componentHandler struct {
	path    string
	origins map[string]bool
	pending sync.Mutex
}

const componentEntryLimit = 500

type storedComponentEntry struct {
	raw        map[string]json.RawMessage
	id         string
	revision   float64
	hasReceipt bool
	receiptKey string
	receiptReq string
}

func newComponentHandler(path string, origins []string) *componentHandler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &componentHandler{path: path, origins: allowed}
}

func (handler *componentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if !handler.origins[r.Header.Get("Origin")] {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "Wrong origin"})
		return
	}
	handler.pending.Lock()
	defer handler.pending.Unlock()
	w.Header().Set("cache-control", "no-store")
	status, body := handler.dispatch(r)
	writeJSON(w, status, body)
}

func (handler *componentHandler) dispatch(r *http.Request) (int, map[string]any) {
	action := filepath.Base(r.URL.Path)
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024+1))
	if err != nil || len(body) == 0 {
		return componentError("Missing component request", http.StatusBadRequest)
	}
	if len(body) > 256*1024 {
		return componentError("Component request is too large", http.StatusRequestEntityTooLarge)
	}
	var input struct {
		ID             string          `json:"id"`
		Revision       *float64        `json:"revision"`
		Definition     json.RawMessage `json:"definition"`
		Circuit        json.RawMessage `json:"circuit"`
		IdempotencyKey string          `json:"idempotencyKey"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return componentError("Missing component request", http.StatusBadRequest)
	}
	entries, err := handler.load()
	if err != nil {
		return componentError(err.Error(), http.StatusBadRequest)
	}
	if action == "list" {
		publicEntries := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			publicEntries = append(publicEntries, entry.public())
		}
		return http.StatusOK, map[string]any{"entries": publicEntries}
	}
	if input.ID == "" {
		return componentError("Component identity required", http.StatusBadRequest)
	}
	var previous *storedComponentEntry
	for index := range entries {
		if entries[index].id == input.ID {
			previous = &entries[index]
			break
		}
	}
	switch action {
	case "read":
		if previous == nil {
			return componentError("Local component not found", http.StatusNotFound)
		}
		return http.StatusOK, map[string]any{"entry": previous.public()}
	case "save", "delete", "restore", "check":
	default:
		return componentError("Unknown component operation", http.StatusNotFound)
	}
	expectedRevision := 0.0
	if previous != nil {
		expectedRevision = previous.revision
	}
	fingerprint := sha256Hex(mustJSON([]any{action, input.Revision, componentPayload(input.Definition, input.Circuit)}))
	if action == "save" && input.IdempotencyKey != "" && previous != nil && previous.hasReceipt && previous.receiptKey == input.IdempotencyKey {
		if previous.receiptReq != fingerprint {
			return componentError("This save identity was used for different content", http.StatusConflict)
		}
		return http.StatusOK, map[string]any{"entry": previous.public()}
	}
	if input.Revision == nil || *input.Revision != expectedRevision {
		return componentError("This local component changed. Reopen it before saving or placing.", http.StatusConflict)
	}
	if action == "check" {
		if previous == nil || entryStatus(previous) == "deleted" {
			return componentError("Local component is no longer available", http.StatusConflict)
		}
		return http.StatusOK, map[string]any{"entry": previous.public()}
	}
	if action != "save" && previous == nil {
		return componentError("Local component not found", http.StatusNotFound)
	}
	if action == "save" && previous != nil && entryStatus(previous) == "deleted" {
		return componentError("Restore this component before editing", http.StatusConflict)
	}
	entry := map[string]any{}
	if previous != nil {
		for _, field := range []string{"authorId", "author", "createdAt", "definition", "circuit"} {
			if value, ok := previous.raw[field]; ok {
				entry[field] = json.RawMessage(value)
			}
		}
	}
	if _, ok := entry["authorId"]; !ok {
		entry["authorId"] = "desktop-local"
	}
	if _, ok := entry["author"]; !ok {
		entry["author"] = "This computer"
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	if _, ok := entry["createdAt"]; !ok {
		entry["createdAt"] = now
	}
	entry["updatedAt"] = now
	entry["id"] = input.ID
	entry["revision"] = int(expectedRevision) + 1
	if action == "save" {
		if len(input.Definition) == 0 || string(input.Definition) == "null" {
			return componentError("Component content required", http.StatusBadRequest)
		}
		entry["status"] = "shared"
		entry["definition"] = json.RawMessage(input.Definition)
		if len(input.Circuit) > 0 && string(input.Circuit) != "null" {
			entry["circuit"] = json.RawMessage(input.Circuit)
		}
		if input.IdempotencyKey != "" {
			entry["receipt"] = map[string]any{"key": input.IdempotencyKey, "request": fingerprint}
		}
	} else {
		if previous == nil || len(previous.raw["definition"]) == 0 {
			return componentError("Component content required", http.StatusBadRequest)
		}
		if action == "delete" {
			entry["status"] = "deleted"
		} else {
			entry["status"] = "shared"
		}
	}
	kept := make([]map[string]json.RawMessage, 0, len(entries)+1)
	for _, candidate := range entries {
		if candidate.id != input.ID {
			kept = append(kept, candidate.raw)
		}
	}
	kept = append(kept, rawEntry(entry))
	if err := handler.write(kept); err != nil {
		return componentError(err.Error(), http.StatusBadRequest)
	}
	public := map[string]any{}
	for field, value := range entry {
		if field == "receipt" {
			continue
		}
		public[field] = value
	}
	return http.StatusOK, map[string]any{"entry": public}
}

func rawEntry(entry map[string]any) map[string]json.RawMessage {
	raw := make(map[string]json.RawMessage, len(entry))
	for field, value := range entry {
		if rawMessage, ok := value.(json.RawMessage); ok {
			raw[field] = rawMessage
		} else {
			raw[field] = json.RawMessage(mustJSON(value))
		}
	}
	return raw
}

func componentPayload(definition json.RawMessage, circuit json.RawMessage) []byte {
	payload := map[string]json.RawMessage{"definition": definition}
	if len(circuit) > 0 && string(circuit) != "null" {
		payload["circuit"] = circuit
	}
	return mustJSON(payload)
}

func entryStatus(entry *storedComponentEntry) string {
	var status string
	if err := json.Unmarshal(entry.raw["status"], &status); err != nil {
		return ""
	}
	return status
}

func (entry *storedComponentEntry) public() map[string]any {
	public := map[string]any{}
	for field, value := range entry.raw {
		if field == "receipt" {
			continue
		}
		public[field] = json.RawMessage(value)
	}
	return public
}

func componentError(message string, status int) (int, map[string]any) {
	return status, map[string]any{"error": message}
}

func (handler *componentHandler) load() ([]storedComponentEntry, error) {
	bytes, err := readLimited(handler.path, maxProjectBytes)
	if isEnoent(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stored struct {
		Format  int               `json:"format"`
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(bytes, &stored); err != nil {
		return nil, errors.New("Local component library is damaged")
	}
	if stored.Format != 1 || len(stored.Entries) > componentEntryLimit {
		return nil, errors.New("Local component library is damaged")
	}
	entries := make([]storedComponentEntry, 0, len(stored.Entries))
	seen := map[string]bool{}
	for _, raw := range stored.Entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, errors.New("Local component library is damaged")
		}
		entry := storedComponentEntry{raw: fields}
		if err := json.Unmarshal(fields["id"], &entry.id); err != nil || entry.id == "" {
			return nil, errors.New("Local component library is damaged")
		}
		if err := json.Unmarshal(fields["revision"], &entry.revision); err != nil {
			return nil, errors.New("Local component library is damaged")
		}
		if seen[entry.id] {
			return nil, errors.New("Duplicate local component identities. Restore the library from a backup.")
		}
		seen[entry.id] = true
		if receiptBytes, ok := fields["receipt"]; ok {
			var receipt struct {
				Key     string `json:"key"`
				Request string `json:"request"`
			}
			if json.Unmarshal(receiptBytes, &receipt) == nil && receipt.Key != "" && receipt.Request != "" {
				entry.hasReceipt = true
				entry.receiptKey = receipt.Key
				entry.receiptReq = receipt.Request
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (handler *componentHandler) write(entries []map[string]json.RawMessage) error {
	text, err := json.Marshal(map[string]any{"format": 1, "entries": entries})
	if err != nil {
		return err
	}
	if len(entries) > componentEntryLimit || len(text) > maxProjectBytes {
		return errors.New("The local component library has reached its storage limit, including removed components. Back up components.json before managing the library outside the application.")
	}
	if err := os.MkdirAll(filepath.Dir(handler.path), 0o755); err != nil {
		return err
	}
	return writeDurable(handler.path, text)
}
