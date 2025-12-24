// Package eval provides .envrc file evaluation via bash subprocess.
package eval

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/unrss/cascade/internal/env"
)

// DumpJSON outputs the environment as JSON to the writer.
// Format: {"KEY": "value", ...}
// Called by stdlib.sh via "cascade dump json".
func DumpJSON(e env.Env, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(e); err != nil {
		return fmt.Errorf("encode env json: %w", err)
	}
	return nil
}

// ParseJSON parses JSON environment output into an Env map.
func ParseJSON(r io.Reader) (env.Env, error) {
	var result env.Env
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode env json: %w", err)
	}
	return result, nil
}
