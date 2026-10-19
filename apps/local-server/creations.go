package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// Durable consent to reopen selected files. Recent shortcuts are only a view:
// removing or evicting one must never revoke an open workspace's file target.
// Port of apps/desktop/src/project-authorizations.ts.
type projectAuthorizations struct {
	path  string
	paths map[string]bool
}

func newProjectAuthorizations(path string) *projectAuthorizations {
	return &projectAuthorizations{path: path}
}

func (authorizations *projectAuthorizations) load() (map[string]bool, error) {
	if authorizations.paths != nil {
		return authorizations.paths, nil
	}
	if authorizations.path == "" {
		authorizations.paths = map[string]bool{}
		return authorizations.paths, nil
	}
	bytes, err := readLimited(authorizations.path, maxProjectBytes)
	if isEnoent(err) {
		authorizations.paths = map[string]bool{}
		return authorizations.paths, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []string
	if err := json.Unmarshal(bytes, &entries); err != nil {
		return nil, errors.New("Saved file authorizations are damaged; reopen files explicitly")
	}
	paths := map[string]bool{}
	for _, entry := range entries {
		if !filepath.IsAbs(entry) {
			return nil, errors.New("Saved file authorizations are damaged; reopen files explicitly")
		}
		paths[pathKey(entry)] = true
	}
	authorizations.paths = paths
	return paths, nil
}

func (authorizations *projectAuthorizations) allows(file string) bool {
	paths, err := authorizations.load()
	if err != nil {
		return false
	}
	return paths[pathKey(file)]
}

func (authorizations *projectAuthorizations) remember(file string) error {
	entries, err := authorizations.load()
	if err != nil {
		if authorizations.path != "" {
			// Preserve the damaged index for inspection, then rebuild.
			os.Rename(authorizations.path, authorizations.path+"."+newUUID()+".damaged")
		}
		entries = map[string]bool{}
	}
	key := pathKey(file)
	if entries[key] {
		return nil
	}
	entries[key] = true
	if authorizations.path != "" {
		list := make([]string, 0, len(entries))
		for entry := range entries {
			list = append(list, entry)
		}
		// Windows path keys are folded; record the real spelling where known.
		if pathKey(file) != file {
			for index := range list {
				if list[index] == key {
					list[index] = file
				}
			}
		}
		sort.Strings(list)
		if err := writeDurable(authorizations.path, mustJSON(list)); err != nil {
			return err
		}
	}
	authorizations.paths = entries
	return nil
}

// A native destination decision survives a lost response: record it after
// overwrite protection, before replacing the file, so retries never choose a
// new destination. Port of apps/desktop/src/project-creations.ts.
type creationRecord struct {
	Path   string  `json:"path"`
	Before *string `json:"before"`
	After  string  `json:"after"`
}

type creationResolution struct {
	newDestination bool
	path           string
	committed      bool
	expected       []byte
}

type projectCreations struct {
	directory string
	memory    map[string]creationRecord
}

func newProjectCreations(directory string) *projectCreations {
	return &projectCreations{directory: directory, memory: map[string]creationRecord{}}
}

func (creations *projectCreations) recordPath(key string) string {
	return filepath.Join(creations.directory, sha256Hex([]byte(key))+".json")
}

func (creations *projectCreations) load(key string) (*creationRecord, error) {
	if creations.directory == "" {
		if record, ok := creations.memory[key]; ok {
			return &record, nil
		}
		return nil, nil
	}
	bytes, err := readLimited(creations.recordPath(key), 32768)
	if isEnoent(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record creationRecord
	if err := json.Unmarshal(bytes, &record); err != nil ||
		!filepath.IsAbs(record.Path) ||
		!isDigest(record.After) ||
		(record.Before != nil && !isDigest(*record.Before)) {
		return nil, errors.New("Creation receipt is damaged; retained files were not changed")
	}
	return &record, nil
}

func isDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

// lookup resolves a retry of a creation. Without a recovery prompt the
// browser host cannot choose a fresh destination, so an interrupted creation
// either replays against unchanged bytes or fails with its retained content.
func (creations *projectCreations) lookup(key string, bytes []byte) (*creationResolution, error) {
	record, err := creations.load(key)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	expected, readErr := readLimited(record.Path, maxProjectBytes)
	unreadable := readErr != nil && !isEnoent(readErr)
	if readErr != nil && !isEnoent(readErr) {
		expected = nil
	}
	current := ""
	if expected != nil {
		current = sha256Hex(expected)
	}
	matches := record.After == sha256Hex(bytes)
	changed := unreadable || (current != record.After && (record.Before == nil || current != *record.Before))
	committed := !unreadable && expected != nil && current == record.After
	if changed {
		return nil, errors.New("The creation destination changed or cannot be read; no file was overwritten")
	}
	if !matches {
		return nil, errors.New("This creation already has retained content. Open the saved Project before making another copy.")
	}
	return &creationResolution{newDestination: false, path: record.Path, committed: committed, expected: expected}, nil
}

func (creations *projectCreations) reserve(key string, path string, before []byte, after []byte, replace bool) error {
	existing, err := creations.load(key)
	if err != nil {
		return err
	}
	if existing != nil {
		if !replace {
			return nil
		}
		if creations.directory != "" {
			os.Rename(creations.recordPath(key), creations.recordPath(key)+"."+newUUID()+".abandoned")
		}
	}
	record := creationRecord{Path: path, After: sha256Hex(after)}
	if before != nil {
		digest := sha256Hex(before)
		record.Before = &digest
	}
	if creations.directory != "" {
		if err := os.MkdirAll(creations.directory, 0o755); err != nil {
			return err
		}
		return writeDurable(creations.recordPath(key), mustJSON(record))
	}
	creations.memory[key] = record
	return nil
}
