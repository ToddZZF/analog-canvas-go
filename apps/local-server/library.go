package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The library owns the on-disk project tree: one <uuid> directory per project
// holding project.icproj.json, its metadata, history and creation receipts.
// All operations run behind the project-file handler's single operation lock.
// Port of apps/desktop/src/project-library.ts.
type libraryRoot struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type librarySettings struct {
	Active string        `json:"active"`
	Roots  []libraryRoot `json:"roots"`
}

type libraryProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Modified int64  `json:"modified"`
	Error    string `json:"error,omitempty"`
	Warning  string `json:"warning,omitempty"`
	Favorite bool   `json:"favorite"`
	Recycled bool   `json:"recycled"`
}

type libraryListing struct {
	Root     string           `json:"root"`
	Roots    []libraryRoot    `json:"roots"`
	Projects []libraryProject `json:"projects"`
}

type projectLibrary struct {
	stateDirectory string
	defaultRoot    string
	settings       *librarySettings
}

func newProjectLibrary(stateDirectory string, defaultRoot string) *projectLibrary {
	return &projectLibrary{stateDirectory: stateDirectory, defaultRoot: defaultRoot}
}

func (library *projectLibrary) settingsPath() string {
	return filepath.Join(library.stateDirectory, "project-library.json")
}

func (library *projectLibrary) load() (*librarySettings, error) {
	if library.settings != nil {
		return library.settings, nil
	}
	path := library.settingsPath()
	if info, err := os.Lstat(path); err == nil && info.Size() > 1024*1024 {
		return nil, errors.New("Project library settings exceed the size limit")
	}
	bytes, err := readLimited(path, 1024*1024)
	switch {
	case isEnoent(err):
		if err := os.MkdirAll(library.defaultRoot, 0o755); err != nil {
			return nil, err
		}
		rootPath, err := filepath.EvalSymlinks(library.defaultRoot)
		if err != nil {
			return nil, err
		}
		root := libraryRoot{ID: newUUID(), Path: rootPath}
		value := &librarySettings{Active: root.ID, Roots: []libraryRoot{root}}
		if err := library.persist(value); err != nil {
			return nil, err
		}
		return value, nil
	case err != nil:
		return nil, err
	}
	var value librarySettings
	if err := json.Unmarshal(bytes, &value); err != nil {
		return nil, errors.New("Project library settings are damaged; choose a project directory")
	}
	if len(value.Roots) == 0 || value.Active == "" {
		return nil, errors.New("Project library settings are damaged; choose a project directory")
	}
	activeSeen := false
	for _, root := range value.Roots {
		if !isUUID(root.ID) || !filepath.IsAbs(root.Path) {
			return nil, errors.New("Project library settings are damaged; choose a project directory")
		}
		if root.ID == value.Active {
			activeSeen = true
		}
	}
	if !activeSeen {
		return nil, errors.New("Project library settings are damaged; choose a project directory")
	}
	library.settings = &value
	return &value, nil
}

func (library *projectLibrary) persist(value *librarySettings) error {
	if err := os.MkdirAll(library.stateDirectory, 0o755); err != nil {
		return err
	}
	if err := writeDurable(library.settingsPath(), mustJSON(value)); err != nil {
		return err
	}
	library.settings = value
	return nil
}

func (library *projectLibrary) activeRoot(value *librarySettings) (libraryRoot, error) {
	for _, root := range value.Roots {
		if root.ID == value.Active {
			return root, nil
		}
	}
	return libraryRoot{}, errors.New("The active project directory is unavailable")
}

// realpath resolves an existing path; the returned error wraps ENOENT when
// the path is missing, mirroring Node's realpath.
func realpath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func moved(err error) bool {
	return err != nil
}

func (library *projectLibrary) list() (*libraryListing, error) {
	value, err := library.load()
	if err != nil {
		return nil, err
	}
	projects := []libraryProject{}
	for _, root := range value.Roots {
		for _, recycled := range []bool{false, true} {
			scanRoot := root.Path
			if recycled {
				scanRoot = filepath.Join(root.Path, ".recycle")
			}
			directories, err := func() ([]os.DirEntry, error) {
				resolved, err := realpath(scanRoot)
				if err != nil || pathKey(resolved) != pathKey(scanRoot) {
					return nil, errors.New("Project directory moved")
				}
				return os.ReadDir(scanRoot)
			}()
			if err != nil {
				if !recycled {
					projects = append(projects, libraryProject{
						ID:    "library:" + root.ID,
						Name:  "Unavailable project directory",
						Path:  root.Path,
						Error: "Locate this directory or choose another one",
					})
				}
				continue
			}
			for _, directory := range directories {
				if !directory.IsDir() || directory.Type()&os.ModeSymlink != 0 || strings.HasPrefix(directory.Name(), ".") {
					continue
				}
				dirPath := filepath.Join(scanRoot, directory.Name())
				path := filepath.Join(dirPath, "project.icproj.json")
				entry := libraryProject{
					ID:       "library:" + root.ID + ":" + map[bool]string{true: "recycle:", false: ""}[recycled] + directory.Name(),
					Name:     directory.Name(),
					Path:     path,
					Favorite: false,
					Recycled: recycled,
				}
				if err := library.inspectEntry(dirPath, path, &entry); err != nil {
					if isEnoent(err) {
						if !isUUID(directory.Name()) {
							continue
						}
						entry.Error = "Project file is missing or its first save was interrupted. Open its folder to inspect retained data."
					} else {
						entry.Error = err.Error()
					}
				}
				if meta, err := readLimited(filepath.Join(dirPath, ".project-meta.json"), 4096); err != nil {
					if !isEnoent(err) {
						entry.Warning = "Project metadata could not be read; content is retained"
					}
				} else {
					var parsed struct {
						Favorite bool `json:"favorite"`
					}
					if json.Unmarshal(meta, &parsed) == nil {
						entry.Favorite = parsed.Favorite
					}
				}
				projects = append(projects, entry)
			}
		}
	}
	sort.SliceStable(projects, func(left, right int) bool {
		return projects[left].Modified > projects[right].Modified
	})
	active, err := library.activeRoot(value)
	if err != nil {
		return nil, err
	}
	return &libraryListing{Root: active.Path, Roots: value.Roots, Projects: projects}, nil
}

// inspectEntry fills modified/name or returns the failure the scanner records.
func (library *projectLibrary) inspectEntry(dirPath string, path string, entry *libraryProject) error {
	resolvedDir, err := realpath(dirPath)
	if moved(err) || pathKey(resolvedDir) != pathKey(dirPath) {
		return errors.New("Project directory moved")
	}
	pending := filepath.Join(dirPath, ".pending-create")
	if marker, err := os.Lstat(pending); err == nil {
		if !marker.Mode().IsRegular() || marker.Mode()&os.ModeSymlink != 0 {
			return errors.New("Invalid creation marker")
		}
		_, wasMissingErr := os.Lstat(path)
		wasMissing := isEnoent(wasMissingErr)
		if !wasMissing && wasMissingErr != nil {
			return wasMissingErr
		}
		if err := newProjectHistory(filepath.Join(dirPath, ".history"), path).recoverCreation(); err != nil {
			return err
		}
		if _, err := os.Lstat(path); err != nil {
			return err
		}
		if err := os.Remove(pending); err != nil {
			return err
		}
		if wasMissing {
			entry.Warning = "Recovered an interrupted first save"
		}
	} else if !isEnoent(err) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProjectBytes {
		return errors.New("Invalid or oversized Project file")
	}
	entry.Modified = info.ModTime().UnixMilli()
	resolvedDir, err = realpath(dirPath)
	if moved(err) || pathKey(resolvedDir) != pathKey(dirPath) {
		return errors.New("Project directory moved")
	}
	bytes, err := readLimited(path, maxProjectBytes)
	if err != nil {
		return err
	}
	document, err := parseProjectDocument(bytes)
	if err != nil {
		return err
	}
	entry.Name = document.name
	return nil
}

func (library *projectLibrary) inspect(id string) (*libraryProject, error) {
	listing, err := library.list()
	if err != nil {
		return nil, err
	}
	for index := range listing.Projects {
		if listing.Projects[index].ID == id {
			if listing.Projects[index].Error != "" {
				return nil, errors.New(listing.Projects[index].Error)
			}
			entry := listing.Projects[index]
			return &entry, nil
		}
	}
	return nil, errors.New("Unknown library Project")
}

func (library *projectLibrary) locate(id string) (string, error) {
	entry, err := library.inspect(id)
	if err != nil {
		return "", err
	}
	return entry.Path, nil
}

func (library *projectLibrary) resolve(id string) (string, error) {
	entry, err := library.inspect(id)
	if err != nil {
		return "", err
	}
	if entry.Recycled {
		return "", errors.New("Restore the Project from the recycle area before opening")
	}
	return entry.Path, nil
}

func (library *projectLibrary) favorite(id string, favorite bool) error {
	entry, err := library.inspect(id)
	if err != nil {
		return err
	}
	if entry.Warning != "" {
		return errors.New(entry.Warning)
	}
	return writeDurable(filepath.Join(filepath.Dir(entry.Path), ".project-meta.json"), mustJSON(struct {
		Favorite bool `json:"favorite"`
	}{favorite}))
}

// recycle moves a project directory into (or out of, with restore) .recycle.
func (library *projectLibrary) recycle(id string, restore bool) error {
	entry, err := library.inspect(id)
	if err != nil {
		return err
	}
	if entry.Recycled != restore {
		return errors.New("Project state changed; refresh the list")
	}
	source := filepath.Dir(entry.Path)
	var base string
	if restore {
		base = filepath.Dir(filepath.Dir(source))
	} else {
		base = filepath.Dir(source)
	}
	targetRoot := base
	if !restore {
		targetRoot = filepath.Join(base, ".recycle")
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return err
	}
	resolved, err := realpath(targetRoot)
	if err != nil || pathKey(resolved) != pathKey(targetRoot) {
		return errors.New("Project directory moved; no Project was moved")
	}
	relativeBase := base
	if restore {
		relativeBase = filepath.Join(base, ".recycle")
	}
	relativePath, err := filepath.Rel(relativeBase, source)
	if err != nil {
		return err
	}
	target := filepath.Join(targetRoot, relativePath)
	if _, err := os.Lstat(target); err == nil {
		return errors.New("A Project already occupies this location; no Project was replaced")
	} else if !isEnoent(err) {
		return err
	}
	return os.Rename(source, target)
}

func (library *projectLibrary) purge(id string) error {
	entry, err := library.inspect(id)
	if err != nil {
		return err
	}
	if !entry.Recycled {
		return errors.New("Only recycled Projects can be removed permanently")
	}
	source := filepath.Dir(entry.Path)
	if filepath.Base(filepath.Dir(source)) != ".recycle" {
		return errors.New("Invalid recycle location")
	}
	return os.RemoveAll(source)
}

// historyDirectory keeps history beside library projects and indexes external
// files under the state directory so opened-anywhere files still get versions.
func (library *projectLibrary) historyDirectory(path string) (string, error) {
	value, err := library.load()
	if err != nil {
		return "", err
	}
	for _, root := range value.Roots {
		relativePath, err := filepath.Rel(root.Path, path)
		if err != nil {
			continue
		}
		segments := strings.Split(relativePath, string(filepath.Separator))
		if len(segments) == 2 && segments[0] != "" && segments[1] == "project.icproj.json" {
			return filepath.Join(filepath.Dir(path), ".history"), nil
		}
	}
	indexPath := filepath.Join(library.stateDirectory, "external-history.json")
	var index []struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}
	if bytes, err := readLimited(indexPath, 1024*1024); err == nil {
		if err := json.Unmarshal(bytes, &index); err != nil {
			return "", errors.New("External history index is damaged")
		}
		for _, entry := range index {
			if !isUUID(entry.ID) {
				return "", errors.New("External history index is damaged")
			}
		}
	} else if !isEnoent(err) {
		return "", err
	}
	for _, entry := range index {
		if pathKey(entry.Path) == pathKey(path) {
			return filepath.Join(library.stateDirectory, "external-history", entry.ID), nil
		}
	}
	entry := struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}{path, newUUID()}
	index = append(index, entry)
	if err := writeDurable(indexPath, mustJSON(index)); err != nil {
		return "", err
	}
	return filepath.Join(library.stateDirectory, "external-history", entry.ID), nil
}

type creationSeed struct {
	Key   string
	Bytes []byte
}

// allocate returns the destination path for a new project file. A creation
// seed makes the allocation deterministic and idempotent for retries.
func (library *projectLibrary) allocate(creation *creationSeed) (string, error) {
	value, err := library.load()
	if err != nil {
		return "", err
	}
	root, err := library.activeRoot(value)
	if err != nil {
		return "", err
	}
	current, err := realpath(root.Path)
	if err != nil {
		return "", err
	}
	if pathKey(current) != pathKey(root.Path) {
		return "", errors.New("Project directory moved; choose its current location")
	}
	var directory string
	if creation != nil {
		if creation.Key == "" || len(creation.Key) > 200 {
			return "", errors.New("Invalid creation identity")
		}
		hash := sha256Hex([]byte(creation.Key))[:32]
		id := hash[0:8] + "-" + hash[8:12] + "-" + hash[12:16] + "-" + hash[16:20] + "-" + hash[20:32]
		digest := sha256Hex(creation.Bytes)
		directory = filepath.Join(current, id)
		for _, candidate := range value.Roots {
			previous := filepath.Join(candidate.Path, id)
			if _, err := os.Lstat(previous); err == nil {
				directory = previous
				break
			} else if !isEnoent(err) {
				return "", err
			}
		}
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return "", err
		}
		resolved, err := realpath(directory)
		if err != nil {
			return "", err
		}
		if pathKey(resolved) != pathKey(directory) {
			return "", errors.New("Creation directory moved")
		}
		record := filepath.Join(directory, ".creation.json")
		recordBytes, err := readLimited(record, 4096)
		switch {
		case err == nil:
			var previous struct {
				Key    string `json:"key"`
				Digest string `json:"digest"`
			}
			if json.Unmarshal(recordBytes, &previous) != nil ||
				previous.Key != creation.Key || previous.Digest != digest {
				return "", errors.New("This creation already has retained content. Open it from Local projects before making another copy.")
			}
		case isEnoent(err):
			contents, err := os.ReadDir(directory)
			if err != nil {
				return "", err
			}
			for _, item := range contents {
				name := item.Name()
				if strings.HasPrefix(name, ".creation.json.") && strings.HasSuffix(name, ".tmp") {
					continue
				}
				return "", errors.New("Incomplete creation contains retained files; inspect its folder before retrying")
			}
			if err := writeDurable(record, mustJSON(struct {
				Key    string `json:"key"`
				Digest string `json:"digest"`
			}{creation.Key, digest})); err != nil {
				return "", err
			}
		default:
			return "", err
		}
		existing, err := readLimited(filepath.Join(directory, "project.icproj.json"), maxProjectBytes)
		switch {
		case err == nil:
			if sha256Hex(existing) != digest {
				return "", errors.New("The previously created Project changed; no file was overwritten")
			}
			return filepath.Join(directory, "project.icproj.json"), nil
		case isEnoent(err):
		default:
			return "", err
		}
	} else {
		directory = filepath.Join(current, newUUID())
		if err := os.Mkdir(directory, 0o755); err != nil {
			return "", err
		}
	}
	if err := writeDurable(filepath.Join(directory, ".pending-create"), []byte("1")); err != nil {
		return "", err
	}
	return filepath.Join(directory, "project.icproj.json"), nil
}

// discardFailedAllocation removes a never-committed allocation directory owned
// by the library; retained receipts keep the location retriable instead.
func (library *projectLibrary) discardFailedAllocation(path string) {
	value, err := library.load()
	if err != nil {
		return
	}
	owner := ""
	directory := filepath.Dir(path)
	for _, root := range value.Roots {
		if filepath.Dir(directory) == root.Path && filepath.Base(path) == "project.icproj.json" && isUUID(filepath.Base(directory)) {
			owner = root.Path
			break
		}
	}
	if owner == "" {
		return
	}
	resolved, err := realpath(directory)
	if err != nil || pathKey(resolved) != pathKey(directory) {
		return
	}
	if _, err := os.Lstat(path); err == nil {
		return
	} else if !isEnoent(err) {
		return
	}
	marker, err := os.Lstat(filepath.Join(directory, ".pending-create"))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode()&os.ModeSymlink != 0 {
		return
	}
	if _, err := os.Lstat(filepath.Join(directory, ".creation.json")); err == nil {
		return
	} else if !isEnoent(err) {
		return
	}
	os.RemoveAll(directory)
}

// archiveReplacement preserves an about-to-be-replaced external project and
// its history inside the recycle area before new bytes take its place.
func (library *projectLibrary) archiveReplacement(path string, bytes []byte) error {
	historyPath, err := library.historyDirectory(path)
	if err != nil {
		return err
	}
	versions, err := newProjectHistory(historyPath, path).list()
	if err != nil {
		return err
	}
	value, err := library.load()
	if err != nil {
		return err
	}
	root, err := library.activeRoot(value)
	if err != nil {
		return err
	}
	canonical, err := realpath(root.Path)
	if err != nil {
		return err
	}
	if pathKey(canonical) != pathKey(root.Path) {
		return errors.New("Project directory moved; choose its current location")
	}
	recycle := filepath.Join(canonical, ".recycle")
	if err := os.MkdirAll(recycle, 0o755); err != nil {
		return err
	}
	resolved, err := realpath(recycle)
	if err != nil || pathKey(resolved) != pathKey(recycle) {
		return errors.New("Invalid recycle directory")
	}
	id := newUUID()
	temporary := filepath.Join(recycle, ".archive-"+id)
	if err := os.MkdirAll(filepath.Join(temporary, ".history"), 0o755); err != nil {
		return err
	}
	if err := writeDurable(filepath.Join(temporary, "project.icproj.json"), bytes); err != nil {
		return err
	}
	manifest := historyManifest{Version: 1, Versions: []historyVersion{}}
	for _, version := range versions {
		if err := writeDurable(filepath.Join(temporary, ".history", version.ID+".icproj.json"), []byte(version.Text)); err != nil {
			return err
		}
		manifest.Versions = append(manifest.Versions, historyVersion{ID: version.ID, SavedAt: version.SavedAt})
	}
	if err := writeDurable(filepath.Join(temporary, ".history", "manifest.json"), mustJSON(&manifest)); err != nil {
		return err
	}
	if err := writeDurable(filepath.Join(temporary, ".project-meta.json"), mustJSON(struct {
		ReplacedPath string `json:"replacedPath"`
		Favorite     bool   `json:"favorite"`
	}{path, false})); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(recycle, id))
}
