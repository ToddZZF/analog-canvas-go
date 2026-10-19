package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Save history journals the previous bytes durably before the Project file is
// replaced. Recovery compares bytes, so a process exit on either side of the
// rename never invents a committed version. Port of project-history.ts.
type historyVersion struct {
	ID      string `json:"id"`
	SavedAt int64  `json:"savedAt"`
}

type historyPending struct {
	Before   *string          `json:"before"`
	After    string           `json:"after"`
	Versions []historyVersion `json:"versions"`
}

type historyManifest struct {
	Version  int              `json:"version"`
	Versions []historyVersion `json:"versions"`
	Pending  *historyPending  `json:"pending,omitempty"`
}

type listedVersion struct {
	ID      string `json:"id"`
	SavedAt int64  `json:"savedAt"`
	Text    string `json:"text"`
}

type projectHistory struct {
	directory   string
	projectPath string
}

func newProjectHistory(directory string, projectPath string) *projectHistory {
	return &projectHistory{directory: directory, projectPath: projectPath}
}

func (history *projectHistory) current() ([]byte, error) {
	bytes, err := readLimited(history.projectPath, maxProjectBytes)
	if isEnoent(err) {
		return nil, nil
	}
	return bytes, err
}

func (history *projectHistory) snapshotPath(id string) string {
	return filepath.Join(history.directory, id+".icproj.json")
}

func (history *projectHistory) manifestPath() string {
	return filepath.Join(history.directory, "manifest.json")
}

// clean removes snapshots no manifest version references; maintenance only.
func (history *projectHistory) clean(manifest *historyManifest) {
	keep := map[string]bool{}
	for _, version := range manifest.Versions {
		keep[version.ID+".icproj.json"] = true
	}
	entries, err := os.ReadDir(history.directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) == 36+len(".icproj.json") && name[36:] == ".icproj.json" && isUUID(name[:36]) && !keep[name] {
			os.Remove(filepath.Join(history.directory, name))
		}
	}
}

func validVersions(value []historyVersion) bool {
	if len(value) > 3 {
		return false
	}
	for _, version := range value {
		if !isUUID(version.ID) || version.SavedAt == 0 {
			return false
		}
	}
	return true
}

func (history *projectHistory) load(recoverCreation bool) (*historyManifest, error) {
	bytes, err := readLimited(history.manifestPath(), 16384)
	if isEnoent(err) {
		manifest := &historyManifest{Version: 1, Versions: []historyVersion{}}
		return manifest, nil
	}
	if err != nil {
		return nil, err
	}
	var manifest historyManifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return nil, errors.New("Save history is damaged; preserve it before retrying")
	}
	if manifest.Version != 1 || !validVersions(manifest.Versions) {
		return nil, errors.New("Save history is damaged; preserve it before retrying")
	}
	if manifest.Pending != nil {
		pending := manifest.Pending
		if !isUUID(pending.After) ||
			(pending.Before != nil && !isUUID(*pending.Before)) ||
			!validVersions(pending.Versions) {
			return nil, errors.New("Save history is damaged; preserve it before retrying")
		}
		after, err := readLimited(history.snapshotPath(pending.After), maxProjectBytes)
		if err != nil {
			return nil, err
		}
		currentBytes, err := history.current()
		if err != nil {
			return nil, err
		}
		var before []byte
		if pending.Before != nil {
			if before, err = readLimited(history.snapshotPath(*pending.Before), maxProjectBytes); err != nil {
				return nil, err
			}
		}
		switch {
		case recoverCreation && currentBytes == nil && pending.Before == nil:
			if _, err := parseProjectDocument(after); err != nil {
				return nil, err
			}
			if err := writeDurable(history.projectPath, after); err != nil {
				return nil, err
			}
			manifest = historyManifest{Version: 1, Versions: pending.Versions}
		case bytesEqual(currentBytes, after):
			manifest = historyManifest{Version: 1, Versions: pending.Versions}
		case currentBytes == nil && pending.Before == nil,
			currentBytes != nil && pending.Before != nil && bytesEqual(currentBytes, before):
			manifest = historyManifest{Version: 1, Versions: manifest.Versions}
		default:
			return nil, errors.New("An interrupted save conflicts with the current file. History is retained; save a separate copy.")
		}
		if err := writeDurable(history.manifestPath(), mustJSON(&manifest)); err != nil {
			return nil, err
		}
	}
	history.clean(&manifest)
	return &manifest, nil
}

func (history *projectHistory) list() ([]listedVersion, error) {
	manifest, err := history.load(false)
	if err != nil {
		return nil, err
	}
	versions := make([]listedVersion, 0, len(manifest.Versions))
	for _, version := range manifest.Versions {
		text, err := readLimited(history.snapshotPath(version.ID), maxProjectBytes)
		if err != nil {
			return nil, err
		}
		versions = append(versions, listedVersion{ID: version.ID, SavedAt: version.SavedAt, Text: string(text)})
	}
	return versions, nil
}

// prepare journals the transition; finish() resolves it after the rename.
func (history *projectHistory) prepare(before []byte, after []byte, independent bool) error {
	manifest, err := history.load(false)
	if err != nil {
		return err
	}
	if before != nil && bytesEqual(before, after) {
		return nil
	}
	if err := os.MkdirAll(history.directory, 0o755); err != nil {
		return err
	}
	var previous *historyVersion
	if before != nil {
		previous = &historyVersion{ID: newUUID(), SavedAt: time.Now().UnixMilli()}
		if err := writeDurable(history.snapshotPath(previous.ID), before); err != nil {
			return err
		}
	}
	nextID := newUUID()
	if err := writeDurable(history.snapshotPath(nextID), after); err != nil {
		return err
	}
	versions := []historyVersion{}
	if !independent && previous != nil {
		versions = append([]historyVersion{*previous}, manifest.Versions...)
		if len(versions) > 3 {
			versions = versions[:3]
		}
	}
	manifest.Pending = &historyPending{After: nextID, Versions: versions}
	if previous != nil {
		manifest.Pending.Before = &previous.ID
	}
	return writeDurable(history.manifestPath(), mustJSON(manifest))
}

func (history *projectHistory) finish() error {
	_, err := history.load(false)
	return err
}

func (history *projectHistory) recoverCreation() error {
	_, err := history.load(true)
	return err
}
