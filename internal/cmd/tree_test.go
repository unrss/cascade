package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFilterVariables(t *testing.T) {
	tests := []struct {
		name       string
		vars       []VarEntry
		filterVars []string
		want       []string // expected variable names in result
	}{
		{
			name:       "nil filter returns all",
			vars:       []VarEntry{{Name: "A"}, {Name: "B"}, {Name: "C"}},
			filterVars: nil,
			want:       []string{"A", "B", "C"},
		},
		{
			name:       "empty filter returns all",
			vars:       []VarEntry{{Name: "A"}, {Name: "B"}},
			filterVars: []string{},
			want:       []string{"A", "B"},
		},
		{
			name:       "filter to one",
			vars:       []VarEntry{{Name: "A"}, {Name: "B"}, {Name: "C"}},
			filterVars: []string{"B"},
			want:       []string{"B"},
		},
		{
			name:       "filter to multiple",
			vars:       []VarEntry{{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}},
			filterVars: []string{"A", "C"},
			want:       []string{"A", "C"},
		},
		{
			name:       "filter with non-existent variable",
			vars:       []VarEntry{{Name: "A"}, {Name: "B"}},
			filterVars: []string{"X", "Y"},
			want:       []string{},
		},
		{
			name:       "filter with partial match",
			vars:       []VarEntry{{Name: "A"}, {Name: "B"}, {Name: "C"}},
			filterVars: []string{"A", "X"},
			want:       []string{"A"},
		},
		{
			name:       "empty vars returns empty",
			vars:       []VarEntry{},
			filterVars: []string{"A"},
			want:       []string{},
		},
		{
			name:       "preserves order from vars",
			vars:       []VarEntry{{Name: "Z"}, {Name: "A"}, {Name: "M"}},
			filterVars: []string{"M", "Z"},
			want:       []string{"Z", "M"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterVariables(tt.vars, tt.filterVars)
			if len(got) != len(tt.want) {
				t.Errorf("filterVariables() returned %d items, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v.Name != tt.want[i] {
					t.Errorf("filterVariables()[%d].Name = %q, want %q", i, v.Name, tt.want[i])
				}
			}
		})
	}
}

func TestTreeIsPathLikeVar(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Known path-like variables
		{"PATH", true},
		{"MANPATH", true},
		{"INFOPATH", true},
		{"LD_LIBRARY_PATH", true},
		{"LIBRARY_PATH", true},
		{"CPATH", true},
		{"PKG_CONFIG_PATH", true},
		{"PYTHONPATH", true},
		{"GOPATH", true},
		{"NODE_PATH", true},
		{"CLASSPATH", true},
		{"CDPATH", true},

		// Non-path variables
		{"HOME", false},
		{"USER", false},
		{"SHELL", false},
		{"DATABASE_URL", false},
		{"API_KEY", false},
		{"FOO", false},
		{"", false},

		// Similar but not path-like
		{"MYPATH", false},
		{"PATH_TO_FILE", false},
		{"SOME_PATH", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := treeIsPathLikeVar(tt.name); got != tt.want {
				t.Errorf("treeIsPathLikeVar(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestTreeDetectPathAction(t *testing.T) {
	tests := []struct {
		name   string
		oldVal string
		newVal string
		want   string
	}{
		// Set (empty old value)
		{
			name:   "set from empty",
			oldVal: "",
			newVal: "/usr/bin",
			want:   "set",
		},

		// Prepend
		{
			name:   "prepend single path",
			oldVal: "/usr/bin",
			newVal: "/new:/usr/bin",
			want:   "prepend",
		},
		{
			name:   "prepend multiple paths",
			oldVal: "/usr/bin:/usr/local/bin",
			newVal: "/new:/usr/bin:/usr/local/bin",
			want:   "prepend",
		},

		// Append
		{
			name:   "append single path",
			oldVal: "/usr/bin",
			newVal: "/usr/bin:/new",
			want:   "append",
		},
		{
			name:   "append multiple paths",
			oldVal: "/usr/bin:/usr/local/bin",
			newVal: "/usr/bin:/usr/local/bin:/new",
			want:   "append",
		},

		// Modify (both prepend and append)
		{
			name:   "modify both ends",
			oldVal: "/usr/bin",
			newVal: "/before:/usr/bin:/after",
			want:   "modify",
		},

		// Override (completely different)
		{
			name:   "override completely different",
			oldVal: "/usr/bin",
			newVal: "/completely/different",
			want:   "override",
		},
		{
			name:   "override partial overlap",
			oldVal: "/usr/bin:/usr/local/bin",
			newVal: "/usr/bin:/other",
			want:   "override",
		},
		{
			name:   "override same length different content",
			oldVal: "/old/path",
			newVal: "/new/path",
			want:   "override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := treeDetectPathAction(tt.oldVal, tt.newVal); got != tt.want {
				t.Errorf("treeDetectPathAction(%q, %q) = %v, want %v", tt.oldVal, tt.newVal, got, tt.want)
			}
		})
	}
}

func TestDetectVariableChanges(t *testing.T) {
	tests := []struct {
		name       string
		before     map[string]string
		after      map[string]string
		showValues bool
		want       []VarEntry
	}{
		{
			name:       "new variable set",
			before:     map[string]string{},
			after:      map[string]string{"FOO": "bar"},
			showValues: false,
			want:       []VarEntry{{Name: "FOO", Action: "set"}},
		},
		{
			name:       "new variable set with value",
			before:     map[string]string{},
			after:      map[string]string{"FOO": "bar"},
			showValues: true,
			want:       []VarEntry{{Name: "FOO", Action: "set", Value: "bar"}},
		},
		{
			name:       "variable unset",
			before:     map[string]string{"FOO": "bar"},
			after:      map[string]string{},
			showValues: false,
			want:       []VarEntry{{Name: "FOO", Action: "unset"}},
		},
		{
			name:       "variable override non-path",
			before:     map[string]string{"FOO": "old"},
			after:      map[string]string{"FOO": "new"},
			showValues: false,
			want:       []VarEntry{{Name: "FOO", Action: "override"}},
		},
		{
			name:       "path prepend",
			before:     map[string]string{"PATH": "/usr/bin"},
			after:      map[string]string{"PATH": "/new:/usr/bin"},
			showValues: false,
			want:       []VarEntry{{Name: "PATH", Action: "prepend"}},
		},
		{
			name:       "path append",
			before:     map[string]string{"PATH": "/usr/bin"},
			after:      map[string]string{"PATH": "/usr/bin:/new"},
			showValues: false,
			want:       []VarEntry{{Name: "PATH", Action: "append"}},
		},
		{
			name:       "no change",
			before:     map[string]string{"FOO": "bar"},
			after:      map[string]string{"FOO": "bar"},
			showValues: false,
			want:       []VarEntry{},
		},
		{
			name:       "multiple changes sorted",
			before:     map[string]string{"ZZZ": "old"},
			after:      map[string]string{"AAA": "new", "ZZZ": "new"},
			showValues: false,
			want: []VarEntry{
				{Name: "AAA", Action: "set"},
				{Name: "ZZZ", Action: "override"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectVariableChanges(tt.before, tt.after, tt.showValues)
			if len(got) != len(tt.want) {
				t.Errorf("detectVariableChanges() returned %d items, want %d", len(got), len(tt.want))
				t.Errorf("got: %+v", got)
				return
			}
			for i, v := range got {
				if v.Name != tt.want[i].Name {
					t.Errorf("detectVariableChanges()[%d].Name = %q, want %q", i, v.Name, tt.want[i].Name)
				}
				if v.Action != tt.want[i].Action {
					t.Errorf("detectVariableChanges()[%d].Action = %q, want %q", i, v.Action, tt.want[i].Action)
				}
				if v.Value != tt.want[i].Value {
					t.Errorf("detectVariableChanges()[%d].Value = %q, want %q", i, v.Value, tt.want[i].Value)
				}
			}
		})
	}
}

func TestFormatActionSymbol(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{"set", "="},
		{"prepend", "+="},
		{"append", "=+"},
		{"override", ":="},
		{"modify", "~="},
		{"unset", "x"},
		{"unknown", "?"},
		{"", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			if got := formatActionSymbol(tt.action); got != tt.want {
				t.Errorf("formatActionSymbol(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestShortenPathList(t *testing.T) {
	home := "/home/user"

	tests := []struct {
		name     string
		pathList string
		want     string
	}{
		{
			name:     "single path with home",
			pathList: "/home/user/bin",
			want:     "~/bin",
		},
		{
			name:     "multiple paths under home",
			pathList: "/home/user/bin:/home/user/.local/bin",
			want:     "~/bin:~/.local/bin",
		},
		{
			name:     "empty path",
			pathList: "",
			want:     "",
		},
		{
			name:     "nested home path",
			pathList: "/home/user/projects/myapp/bin",
			want:     "~/projects/myapp/bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenPathList(tt.pathList, home); got != tt.want {
				t.Errorf("shortenPathList(%q, %q) = %q, want %q", tt.pathList, home, got, tt.want)
			}
		})
	}
}

func TestTreeOutputJSON(t *testing.T) {
	output := &TreeOutput{
		Root:    "/home/user",
		Current: "/home/user/project",
		Levels: []TreeLevel{
			{
				Path:      "/home/user/.envrc",
				Dir:       "/home/user",
				Exists:    true,
				Status:    "allowed",
				IsCurrent: false,
				Variables: []VarEntry{
					{Name: "HOME_VAR", Action: "set", Value: "value"},
				},
			},
			{
				Path:      "/home/user/project/.envrc",
				Dir:       "/home/user/project",
				Exists:    true,
				Status:    "allowed",
				IsCurrent: true,
				Variables: []VarEntry{
					{Name: "PROJECT_VAR", Action: "set"},
				},
			},
		},
		FinalValues: map[string]string{
			"HOME_VAR":    "value",
			"PROJECT_VAR": "project_value",
		},
	}

	var buf bytes.Buffer
	if err := outputTreeJSON(&buf, output); err != nil {
		t.Fatalf("outputTreeJSON() error = %v", err)
	}

	// Parse the JSON back to verify structure
	var parsed TreeOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if parsed.Root != output.Root {
		t.Errorf("Root = %q, want %q", parsed.Root, output.Root)
	}
	if parsed.Current != output.Current {
		t.Errorf("Current = %q, want %q", parsed.Current, output.Current)
	}
	if len(parsed.Levels) != len(output.Levels) {
		t.Errorf("Levels count = %d, want %d", len(parsed.Levels), len(output.Levels))
	}
	if len(parsed.FinalValues) != len(output.FinalValues) {
		t.Errorf("FinalValues count = %d, want %d", len(parsed.FinalValues), len(output.FinalValues))
	}
}

func TestTreeLevelJSONOmitEmpty(t *testing.T) {
	// Test that empty Variables slice is omitted from JSON
	level := TreeLevel{
		Path:      "/home/user/.envrc",
		Dir:       "/home/user",
		Exists:    true,
		Status:    "allowed",
		IsCurrent: false,
		Variables: nil,
	}

	data, err := json.Marshal(level)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonStr := string(data)
	if bytes.Contains(data, []byte(`"variables"`)) {
		t.Errorf("JSON should omit empty variables, got: %s", jsonStr)
	}
}

func TestVarEntryJSONOmitEmpty(t *testing.T) {
	// Test that empty Value is omitted from JSON
	entry := VarEntry{
		Name:   "FOO",
		Action: "set",
		Value:  "",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonStr := string(data)
	if bytes.Contains(data, []byte(`"value"`)) {
		t.Errorf("JSON should omit empty value, got: %s", jsonStr)
	}

	// Test that non-empty Value is included
	entry.Value = "bar"
	data, err = json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if !bytes.Contains(data, []byte(`"value":"bar"`)) {
		t.Errorf("JSON should include non-empty value, got: %s", string(data))
	}
}

func TestTreeOutputFinalValuesOmitEmpty(t *testing.T) {
	// Test that empty FinalValues is omitted from JSON
	output := &TreeOutput{
		Root:        "/home/user",
		Current:     "/home/user",
		Levels:      []TreeLevel{},
		FinalValues: nil,
	}

	var buf bytes.Buffer
	if err := outputTreeJSON(&buf, output); err != nil {
		t.Fatalf("outputTreeJSON() error = %v", err)
	}

	if bytes.Contains(buf.Bytes(), []byte(`"final_values"`)) {
		t.Errorf("JSON should omit empty final_values, got: %s", buf.String())
	}
}
