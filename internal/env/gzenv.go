package env

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

// Marshal encodes an EnvDiff to the gzenv format (JSON → zlib → base64 URL-safe).
// Returns an empty string for nil or empty diffs.
func Marshal(diff *EnvDiff) (string, error) {
	if diff == nil || diff.IsEmpty() {
		return "", nil
	}

	// JSON encode
	jsonData, err := json.Marshal(diff)
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

// Unmarshal decodes a gzenv string back to EnvDiff.
// Returns an empty diff for empty input.
func Unmarshal(gzenv string) (*EnvDiff, error) {
	if gzenv == "" {
		return &EnvDiff{
			Prev: make(map[string]string),
			Next: make(map[string]string),
		}, nil
	}

	// Base64 URL-safe decode
	compressed, err := base64.URLEncoding.DecodeString(gzenv)
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
	var diff EnvDiff
	if err := json.Unmarshal(jsonData, &diff); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}

	// Ensure maps are initialized
	if diff.Prev == nil {
		diff.Prev = make(map[string]string)
	}
	if diff.Next == nil {
		diff.Next = make(map[string]string)
	}

	return &diff, nil
}
