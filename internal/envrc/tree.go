package envrc

import (
	"fmt"
	"path/filepath"
	"strings"
)

const envrcName = ".envrc"

// FindChain discovers all .envrc files from root to target directory.
// Returns ordered slice from root (first) to target (last).
// Includes entries for directories without .envrc (Exists=false) for watch tracking.
//
// Example: FindChain("/home/user", "/home/user/work/api")
// Returns RCs for:
//   - /home/user/.envrc (if exists, or Exists=false)
//   - /home/user/work/.envrc (if exists, or Exists=false)
//   - /home/user/work/api/.envrc (if exists, or Exists=false)
func FindChain(root, target string) ([]*RC, error) {
	// Resolve to absolute paths
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute root path: %w", err)
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return nil, fmt.Errorf("absolute target path: %w", err)
	}

	// Resolve symlinks
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve root symlinks: %w", err)
	}

	absTarget, err = filepath.EvalSymlinks(absTarget)
	if err != nil {
		return nil, fmt.Errorf("resolve target symlinks: %w", err)
	}

	// Ensure target is under root
	if !strings.HasPrefix(absTarget, absRoot) {
		return nil, fmt.Errorf("target %s is not under root %s", absTarget, absRoot)
	}

	// Walk UP from target to root, collecting directories
	var dirs []string
	current := absTarget
	for {
		dirs = append(dirs, current)
		if current == absRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding our root
			return nil, fmt.Errorf("target %s is not under root %s", absTarget, absRoot)
		}
		current = parent
	}

	// Reverse to get root-first order
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}

	// Create RC for each directory
	chain := make([]*RC, 0, len(dirs))
	for _, dir := range dirs {
		envrcPath := filepath.Join(dir, envrcName)
		rc, err := NewRC(envrcPath)
		if err != nil {
			return nil, fmt.Errorf("create RC for %s: %w", envrcPath, err)
		}
		chain = append(chain, rc)
	}

	return chain, nil
}

// ExistingOnly filters to only RCs where Exists=true.
func ExistingOnly(chain []*RC) []*RC {
	result := make([]*RC, 0, len(chain))
	for _, rc := range chain {
		if rc.Exists {
			result = append(result, rc)
		}
	}
	return result
}
