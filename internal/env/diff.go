package env

// EnvDiff represents changes between two environments.
// It captures the minimal information needed to transform one environment
// into another, and to reverse that transformation.
type EnvDiff struct {
	// Prev contains values to restore on revert.
	// For changed keys: the original value from e1.
	// For removed keys: the original value from e1.
	// For added keys: empty string (key didn't exist).
	Prev map[string]string `json:"p"`

	// Next contains values to apply.
	// For changed keys: the new value from e2.
	// For added keys: the new value from e2.
	// For removed keys: empty string (key should be deleted).
	Next map[string]string `json:"n"`
}

// BuildEnvDiff computes the diff from e1 (before) to e2 (after).
// Both environments are filtered to exclude ignored keys before comparison.
// Prev contains e1 values for keys that changed or were removed.
// Next contains e2 values for keys that changed or were added.
func BuildEnvDiff(e1, e2 Env) *EnvDiff {
	f1 := e1.Filtered()
	f2 := e2.Filtered()

	diff := &EnvDiff{
		Prev: make(map[string]string),
		Next: make(map[string]string),
	}

	// Find keys in e1 that changed or were removed
	for key, v1 := range f1 {
		if v2, exists := f2[key]; exists {
			if v1 != v2 {
				// Changed
				diff.Prev[key] = v1
				diff.Next[key] = v2
			}
		} else {
			// Removed
			diff.Prev[key] = v1
			diff.Next[key] = ""
		}
	}

	// Find keys added in e2
	for key, v2 := range f2 {
		if _, exists := f1[key]; !exists {
			// Added
			diff.Prev[key] = ""
			diff.Next[key] = v2
		}
	}

	return diff
}

// Patch applies the diff to an environment (for applying changes).
// Keys with empty values in Next are deleted from the environment.
// Returns a new environment; the original is not modified.
func (d *EnvDiff) Patch(env Env) Env {
	if d == nil {
		return env.Copy()
	}

	result := env.Copy()
	if result == nil {
		result = make(Env)
	}

	for key, value := range d.Next {
		if value == "" {
			delete(result, key)
		} else {
			result[key] = value
		}
	}

	return result
}

// Reverse returns a new diff that undoes this diff.
// Applying the reversed diff restores the original environment.
func (d *EnvDiff) Reverse() *EnvDiff {
	if d == nil {
		return &EnvDiff{
			Prev: make(map[string]string),
			Next: make(map[string]string),
		}
	}

	return &EnvDiff{
		Prev: copyMap(d.Next),
		Next: copyMap(d.Prev),
	}
}

// IsEmpty returns true if no changes are recorded in the diff.
func (d *EnvDiff) IsEmpty() bool {
	if d == nil {
		return true
	}
	return len(d.Next) == 0 && len(d.Prev) == 0
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
