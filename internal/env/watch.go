package env

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// FileTime tracks a file's modification state.
type FileTime struct {
	Path    string `json:"p"` // Absolute path
	Modtime int64  `json:"m"` // Unix timestamp (0 if doesn't exist)
	Exists  bool   `json:"e"` // Whether file existed at check time
}

// NewFileTime creates a FileTime by stat'ing the path.
// Uses os.Stat which follows symlinks.
func NewFileTime(path string) FileTime {
	ft := FileTime{Path: path}

	info, err := os.Stat(path)
	if err != nil {
		// File doesn't exist or can't be accessed
		ft.Exists = false
		ft.Modtime = 0
		return ft
	}

	ft.Exists = true
	ft.Modtime = info.ModTime().Unix()
	return ft
}

// Check returns true if the file has changed since this FileTime was created.
// Changes include: modification, creation, or deletion.
func (ft FileTime) Check() bool {
	current := NewFileTime(ft.Path)

	// Existence changed (created or deleted)
	if ft.Exists != current.Exists {
		return true
	}

	// If file exists, check modtime
	if ft.Exists && ft.Modtime != current.Modtime {
		return true
	}

	return false
}

// WatchList is a collection of files being watched.
type WatchList []FileTime

// NewWatchList creates a WatchList from a list of paths.
func NewWatchList(paths []string) WatchList {
	wl := make(WatchList, len(paths))
	for i, path := range paths {
		wl[i] = NewFileTime(path)
	}
	return wl
}

// Check returns true if any watched file has changed.
func (wl WatchList) Check() bool {
	for _, ft := range wl {
		if ft.Check() {
			return true
		}
	}
	return false
}

// Serialize encodes the WatchList for storage in CASCADE_WATCHES.
// Uses gzenv format (JSON → zlib → base64 URL-safe).
func (wl WatchList) Serialize() (string, error) {
	if len(wl) == 0 {
		return "", nil
	}

	// JSON encode
	jsonData, err := json.Marshal(wl)
	if err != nil {
		return "", fmt.Errorf("json encode: %w", err)
	}

	// Zlib compress
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(jsonData); err != nil {
		return "", fmt.Errorf("zlib write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("zlib close: %w", err)
	}

	// Base64 URL-safe encode
	encoded := base64.URLEncoding.EncodeToString(compressed.Bytes())

	return encoded, nil
}

// ParseWatchList decodes a serialized WatchList.
func ParseWatchList(encoded string) (WatchList, error) {
	if encoded == "" {
		return WatchList{}, nil
	}

	// Base64 URL-safe decode
	compressed, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	// Zlib decompress
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("zlib reader: %w", err)
	}
	defer r.Close()

	jsonData, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("zlib read: %w", err)
	}

	// JSON decode
	var wl WatchList
	if err := json.Unmarshal(jsonData, &wl); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}

	return wl, nil
}
