package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The persisted Project document is owned by @icm/project-protocol. The local
// server only performs the two mutations the desktop shell performs through
// that protocol (an independent copy and a project rename) and reads the two
// fields the library listing needs. Everything else stays opaque bytes the
// renderer revalidates with the real parser on load.
var errInvalidProject = errors.New("This file is not a valid Project document")

type projectDocument struct {
	raw               map[string]json.RawMessage
	id                string
	name              string
	structureRevision *float64
}

func parseProjectDocument(text []byte) (*projectDocument, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(text, &raw); err != nil {
		return nil, errInvalidProject
	}
	document := &projectDocument{raw: raw}
	if value, ok := raw["id"]; ok {
		if err := json.Unmarshal(value, &document.id); err != nil || document.id == "" {
			return nil, errInvalidProject
		}
	} else {
		return nil, errInvalidProject
	}
	if value, ok := raw["name"]; ok {
		if err := json.Unmarshal(value, &document.name); err != nil {
			return nil, errInvalidProject
		}
	}
	if value, ok := raw["structureRevision"]; ok {
		var revision float64
		if err := json.Unmarshal(value, &revision); err == nil {
			document.structureRevision = &revision
		}
	}
	return document, nil
}

// setName applies the rename_project edit: the display name plus one
// structural revision, exactly as packages/edit-engine applies it.
func (document *projectDocument) setName(name string) {
	document.name = name
	document.raw["name"], _ = json.Marshal(name)
	if document.structureRevision != nil {
		*document.structureRevision++
		document.raw["structureRevision"], _ = json.Marshal(*document.structureRevision)
	}
}

// makeIndependent mirrors createIndependentProject: a copy with a fresh
// identity and its own name; saved files keep canonical text from the renderer.
func (document *projectDocument) makeIndependent(id string, name string) {
	document.id = id
	document.name = name
	document.raw["id"], _ = json.Marshal(id)
	document.raw["name"], _ = json.Marshal(name)
}

func (document *projectDocument) serialize() ([]byte, error) {
	return json.Marshal(document.raw)
}

func suffixName(name string, suffix string) string {
	runes := []rune(name)
	capacity := 120 - len([]rune(suffix)) - 3
	if len(runes) > capacity {
		runes = runes[:capacity]
	}
	return fmt.Sprintf("%s (%s)", string(runes), suffix)
}
