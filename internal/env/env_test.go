package env

import (
	"slices"
	"testing"
)

func TestFromGoEnv(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    Env
	}{
		{
			name:    "empty",
			environ: nil,
			want:    Env{},
		},
		{
			name:    "single var",
			environ: []string{"FOO=bar"},
			want:    Env{"FOO": "bar"},
		},
		{
			name:    "multiple vars",
			environ: []string{"FOO=bar", "BAZ=qux"},
			want:    Env{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:    "empty value",
			environ: []string{"FOO="},
			want:    Env{"FOO": ""},
		},
		{
			name:    "value with equals",
			environ: []string{"FOO=bar=baz"},
			want:    Env{"FOO": "bar=baz"},
		},
		{
			name:    "no equals ignored",
			environ: []string{"INVALID", "FOO=bar"},
			want:    Env{"FOO": "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromGoEnv(tt.environ)
			if len(got) != len(tt.want) {
				t.Errorf("FromGoEnv() len = %d, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("FromGoEnv()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestToGoEnv(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		want []string
	}{
		{
			name: "nil",
			env:  nil,
			want: nil,
		},
		{
			name: "empty",
			env:  Env{},
			want: []string{},
		},
		{
			name: "single var",
			env:  Env{"FOO": "bar"},
			want: []string{"FOO=bar"},
		},
		{
			name: "sorted output",
			env:  Env{"ZZZ": "last", "AAA": "first", "MMM": "middle"},
			want: []string{"AAA=first", "MMM=middle", "ZZZ=last"},
		},
		{
			name: "empty value",
			env:  Env{"FOO": ""},
			want: []string{"FOO="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.env.ToGoEnv()
			if !slices.Equal(got, tt.want) {
				t.Errorf("ToGoEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := []string{"FOO=bar", "BAZ=qux", "EMPTY="}
	env := FromGoEnv(original)
	result := env.ToGoEnv()

	slices.Sort(original)
	if !slices.Equal(result, original) {
		t.Errorf("round-trip failed: got %v, want %v", result, original)
	}
}

func TestEnvCopy(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var env Env
		got := env.Copy()
		if got != nil {
			t.Errorf("Copy() of nil = %v, want nil", got)
		}
	})

	t.Run("deep copy", func(t *testing.T) {
		env := Env{"FOO": "bar"}
		cp := env.Copy()

		// Modify original
		env["FOO"] = "modified"
		env["NEW"] = "value"

		// Copy should be unchanged
		if cp["FOO"] != "bar" {
			t.Errorf("Copy was modified: FOO = %q, want %q", cp["FOO"], "bar")
		}
		if _, exists := cp["NEW"]; exists {
			t.Error("Copy has NEW key that was added after copy")
		}
	})
}

func TestEnvFiltered(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var env Env
		got := env.Filtered()
		if got != nil {
			t.Errorf("Filtered() of nil = %v, want nil", got)
		}
	})

	t.Run("removes ignored keys", func(t *testing.T) {
		env := Env{
			"FOO":             "bar",
			"PWD":             "/home/user",
			"OLDPWD":          "/tmp",
			"SHLVL":           "2",
			"_":               "/bin/ls",
			"TERM_SESSION_ID": "abc123",
			"CASCADE_DIFF":    "encoded",
			"CASCADE_DIR":     "/project",
		}
		got := env.Filtered()

		if len(got) != 1 {
			t.Errorf("Filtered() len = %d, want 1", len(got))
		}
		if got["FOO"] != "bar" {
			t.Errorf("Filtered()[FOO] = %q, want %q", got["FOO"], "bar")
		}
	})
}

func TestIgnoredEnv(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PWD", true},
		{"OLDPWD", true},
		{"SHLVL", true},
		{"_", true},
		{"TERM_SESSION_ID", true},
		{"CASCADE_DIFF", true},
		{"CASCADE_DIR", true},
		{"CASCADE_", true},
		{"PATH", false},
		{"HOME", false},
		{"USER", false},
		{"CASCADING", false}, // Not CASCADE_ prefix
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IgnoredEnv(tt.key); got != tt.want {
				t.Errorf("IgnoredEnv(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestBuildEnvDiff(t *testing.T) {
	tests := []struct {
		name     string
		e1       Env
		e2       Env
		wantPrev map[string]string
		wantNext map[string]string
	}{
		{
			name:     "no changes",
			e1:       Env{"FOO": "bar"},
			e2:       Env{"FOO": "bar"},
			wantPrev: map[string]string{},
			wantNext: map[string]string{},
		},
		{
			name:     "addition",
			e1:       Env{},
			e2:       Env{"FOO": "bar"},
			wantPrev: map[string]string{"FOO": ""},
			wantNext: map[string]string{"FOO": "bar"},
		},
		{
			name:     "removal",
			e1:       Env{"FOO": "bar"},
			e2:       Env{},
			wantPrev: map[string]string{"FOO": "bar"},
			wantNext: map[string]string{"FOO": ""},
		},
		{
			name:     "change",
			e1:       Env{"FOO": "old"},
			e2:       Env{"FOO": "new"},
			wantPrev: map[string]string{"FOO": "old"},
			wantNext: map[string]string{"FOO": "new"},
		},
		{
			name:     "mixed operations",
			e1:       Env{"KEEP": "same", "CHANGE": "old", "REMOVE": "gone"},
			e2:       Env{"KEEP": "same", "CHANGE": "new", "ADD": "fresh"},
			wantPrev: map[string]string{"CHANGE": "old", "REMOVE": "gone", "ADD": ""},
			wantNext: map[string]string{"CHANGE": "new", "REMOVE": "", "ADD": "fresh"},
		},
		{
			name:     "ignores filtered keys",
			e1:       Env{"FOO": "bar", "PWD": "/old"},
			e2:       Env{"FOO": "bar", "PWD": "/new"},
			wantPrev: map[string]string{},
			wantNext: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := BuildEnvDiff(tt.e1, tt.e2)

			if len(diff.Prev) != len(tt.wantPrev) {
				t.Errorf("Prev len = %d, want %d", len(diff.Prev), len(tt.wantPrev))
			}
			for k, v := range tt.wantPrev {
				if diff.Prev[k] != v {
					t.Errorf("Prev[%q] = %q, want %q", k, diff.Prev[k], v)
				}
			}

			if len(diff.Next) != len(tt.wantNext) {
				t.Errorf("Next len = %d, want %d", len(diff.Next), len(tt.wantNext))
			}
			for k, v := range tt.wantNext {
				if diff.Next[k] != v {
					t.Errorf("Next[%q] = %q, want %q", k, diff.Next[k], v)
				}
			}
		})
	}
}

func TestEnvDiffPatch(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		diff *EnvDiff
		want Env
	}{
		{
			name: "nil diff",
			env:  Env{"FOO": "bar"},
			diff: nil,
			want: Env{"FOO": "bar"},
		},
		{
			name: "add key",
			env:  Env{"FOO": "bar"},
			diff: &EnvDiff{
				Prev: map[string]string{"NEW": ""},
				Next: map[string]string{"NEW": "value"},
			},
			want: Env{"FOO": "bar", "NEW": "value"},
		},
		{
			name: "remove key",
			env:  Env{"FOO": "bar", "REMOVE": "me"},
			diff: &EnvDiff{
				Prev: map[string]string{"REMOVE": "me"},
				Next: map[string]string{"REMOVE": ""},
			},
			want: Env{"FOO": "bar"},
		},
		{
			name: "change key",
			env:  Env{"FOO": "old"},
			diff: &EnvDiff{
				Prev: map[string]string{"FOO": "old"},
				Next: map[string]string{"FOO": "new"},
			},
			want: Env{"FOO": "new"},
		},
		{
			name: "patch nil env",
			env:  nil,
			diff: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			want: Env{"FOO": "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.diff.Patch(tt.env)

			if len(got) != len(tt.want) {
				t.Errorf("Patch() len = %d, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("Patch()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}

	t.Run("does not modify original", func(t *testing.T) {
		env := Env{"FOO": "bar"}
		diff := &EnvDiff{
			Prev: map[string]string{"FOO": "bar"},
			Next: map[string]string{"FOO": "modified"},
		}
		_ = diff.Patch(env)

		if env["FOO"] != "bar" {
			t.Errorf("original env was modified: FOO = %q, want %q", env["FOO"], "bar")
		}
	})
}

func TestEnvDiffReverse(t *testing.T) {
	t.Run("nil diff", func(t *testing.T) {
		var diff *EnvDiff
		rev := diff.Reverse()
		if rev == nil {
			t.Fatal("Reverse() of nil returned nil")
		}
		if len(rev.Prev) != 0 || len(rev.Next) != 0 {
			t.Errorf("Reverse() of nil not empty: Prev=%v, Next=%v", rev.Prev, rev.Next)
		}
	})

	t.Run("swaps prev and next", func(t *testing.T) {
		diff := &EnvDiff{
			Prev: map[string]string{"FOO": "old", "BAR": ""},
			Next: map[string]string{"FOO": "new", "BAR": "added"},
		}
		rev := diff.Reverse()

		if rev.Prev["FOO"] != "new" || rev.Prev["BAR"] != "added" {
			t.Errorf("Reverse().Prev = %v, want map[FOO:new BAR:added]", rev.Prev)
		}
		if rev.Next["FOO"] != "old" || rev.Next["BAR"] != "" {
			t.Errorf("Reverse().Next = %v, want map[FOO:old BAR:]", rev.Next)
		}
	})

	t.Run("apply and reverse restores original", func(t *testing.T) {
		original := Env{"FOO": "bar", "KEEP": "same"}
		diff := &EnvDiff{
			Prev: map[string]string{"FOO": "bar", "NEW": ""},
			Next: map[string]string{"FOO": "changed", "NEW": "added"},
		}

		patched := diff.Patch(original)
		restored := diff.Reverse().Patch(patched)

		if len(restored) != len(original) {
			t.Errorf("restored len = %d, want %d", len(restored), len(original))
		}
		for k, v := range original {
			if restored[k] != v {
				t.Errorf("restored[%q] = %q, want %q", k, restored[k], v)
			}
		}
	})
}

func TestEnvDiffIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		diff *EnvDiff
		want bool
	}{
		{
			name: "nil",
			diff: nil,
			want: true,
		},
		{
			name: "empty maps",
			diff: &EnvDiff{
				Prev: map[string]string{},
				Next: map[string]string{},
			},
			want: true,
		},
		{
			name: "has prev",
			diff: &EnvDiff{
				Prev: map[string]string{"FOO": "bar"},
				Next: map[string]string{},
			},
			want: false,
		},
		{
			name: "has next",
			diff: &EnvDiff{
				Prev: map[string]string{},
				Next: map[string]string{"FOO": "bar"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.diff.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	t.Run("nil diff", func(t *testing.T) {
		encoded, err := Marshal(nil)
		if err != nil {
			t.Fatalf("Marshal(nil) error: %v", err)
		}
		if encoded != "" {
			t.Errorf("Marshal(nil) = %q, want empty", encoded)
		}
	})

	t.Run("empty diff", func(t *testing.T) {
		diff := &EnvDiff{
			Prev: map[string]string{},
			Next: map[string]string{},
		}
		encoded, err := Marshal(diff)
		if err != nil {
			t.Fatalf("Marshal(empty) error: %v", err)
		}
		if encoded != "" {
			t.Errorf("Marshal(empty) = %q, want empty", encoded)
		}
	})

	t.Run("empty string unmarshal", func(t *testing.T) {
		diff, err := Unmarshal("")
		if err != nil {
			t.Fatalf("Unmarshal(\"\") error: %v", err)
		}
		if diff == nil {
			t.Fatal("Unmarshal(\"\") returned nil")
		}
		if len(diff.Prev) != 0 || len(diff.Next) != 0 {
			t.Errorf("Unmarshal(\"\") not empty: Prev=%v, Next=%v", diff.Prev, diff.Next)
		}
	})

	t.Run("round-trip", func(t *testing.T) {
		original := &EnvDiff{
			Prev: map[string]string{"FOO": "old", "REMOVED": "value", "ADDED": ""},
			Next: map[string]string{"FOO": "new", "REMOVED": "", "ADDED": "fresh"},
		}

		encoded, err := Marshal(original)
		if err != nil {
			t.Fatalf("Marshal() error: %v", err)
		}
		if encoded == "" {
			t.Fatal("Marshal() returned empty string for non-empty diff")
		}

		decoded, err := Unmarshal(encoded)
		if err != nil {
			t.Fatalf("Unmarshal() error: %v", err)
		}

		if len(decoded.Prev) != len(original.Prev) {
			t.Errorf("Prev len = %d, want %d", len(decoded.Prev), len(original.Prev))
		}
		for k, v := range original.Prev {
			if decoded.Prev[k] != v {
				t.Errorf("Prev[%q] = %q, want %q", k, decoded.Prev[k], v)
			}
		}

		if len(decoded.Next) != len(original.Next) {
			t.Errorf("Next len = %d, want %d", len(decoded.Next), len(original.Next))
		}
		for k, v := range original.Next {
			if decoded.Next[k] != v {
				t.Errorf("Next[%q] = %q, want %q", k, decoded.Next[k], v)
			}
		}
	})

	t.Run("special characters", func(t *testing.T) {
		original := &EnvDiff{
			Prev: map[string]string{"PATH": "/usr/bin:/bin"},
			Next: map[string]string{"PATH": "/home/user/bin:/usr/bin:/bin"},
		}

		encoded, err := Marshal(original)
		if err != nil {
			t.Fatalf("Marshal() error: %v", err)
		}

		decoded, err := Unmarshal(encoded)
		if err != nil {
			t.Fatalf("Unmarshal() error: %v", err)
		}

		if decoded.Next["PATH"] != original.Next["PATH"] {
			t.Errorf("PATH = %q, want %q", decoded.Next["PATH"], original.Next["PATH"])
		}
	})
}

func TestUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid base64",
			input: "not-valid-base64!!!",
		},
		{
			name:  "valid base64 but not zlib",
			input: "aGVsbG8gd29ybGQ=", // "hello world" in base64
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal(tt.input)
			if err == nil {
				t.Error("Unmarshal() expected error, got nil")
			}
		})
	}
}
