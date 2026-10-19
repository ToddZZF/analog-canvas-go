package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var uuidShape = regexp.MustCompile(`^[a-f0-9-]+$`)

// Byte and request limits mirror apps/desktop/src/project-files.ts and
// durable-files.ts so the browser host accepts exactly the same payloads.
const maxProjectBytes = 16 * 1024 * 1024

var errFileTooLarge = errors.New("File exceeds its size limit")

// readLimited reads at most maximum bytes and refuses files whose size changed
// during the read rather than serving a truncated image of them.
func readLimited(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, errFileTooLarge
	}
	buffer := make([]byte, info.Size())
	if _, err := io.ReadFull(file, buffer); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, errors.New("File changed while reading; retry")
		}
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != info.Size() {
		return nil, errors.New("File changed while reading; retry")
	}
	return buffer, nil
}

// writeDurable replaces path via a unique temp file that is fsynced before the
// rename, so a crash leaves either the old or the new bytes, never a mixture.
func writeDurable(path string, data []byte) error {
	temporary := fmt.Sprintf("%s.%s.tmp", path, newUUID())
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
		os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newUUID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	value := hex.EncodeToString(bytes[:])
	return value[0:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:32]
}

func isUUID(value string) bool {
	return len(value) == 36 && uuidShape.MatchString(value)
}

// pathKey is the case-folding identity Windows applies to its paths.
func pathKey(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func basename(path string) string {
	return filepath.Base(path)
}

func isEnoent(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// sameBytes mirrors the TS same(): nil means "file absent", and an absent
// file only equals another absent file.
func sameBytes(a []byte, b []byte) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return string(a) == string(b)
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic("unmarshalable server state: " + err.Error())
	}
	return data
}
