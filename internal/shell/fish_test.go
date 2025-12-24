package shell

import (
	"strings"
	"testing"
)

func TestFishName(t *testing.T) {
	if got := Fish.Name(); got != "fish" {
		t.Errorf("Name() = %q, want %q", got, "fish")
	}
}

func TestFishHook(t *testing.T) {
	hook := Fish.Hook("/usr/local/bin/cascade")

	t.Run("contains __cascade_export_eval function", func(t *testing.T) {
		if !strings.Contains(hook, "__cascade_export_eval") {
			t.Error("hook should contain __cascade_export_eval function")
		}
	})

	t.Run("uses --on-event fish_prompt", func(t *testing.T) {
		if !strings.Contains(hook, "--on-event fish_prompt") {
			t.Error("hook should use --on-event fish_prompt")
		}
	})

	t.Run("contains __cascade_cd_hook function", func(t *testing.T) {
		if !strings.Contains(hook, "__cascade_cd_hook") {
			t.Error("hook should contain __cascade_cd_hook function")
		}
	})

	t.Run("uses --on-variable PWD", func(t *testing.T) {
		if !strings.Contains(hook, "--on-variable PWD") {
			t.Error("hook should use --on-variable PWD for directory changes")
		}
	})

	t.Run("contains selfPath", func(t *testing.T) {
		if !strings.Contains(hook, "/usr/local/bin/cascade") {
			t.Error("hook should contain the selfPath")
		}
	})

	t.Run("checks CASCADE_FISH_MODE", func(t *testing.T) {
		if !strings.Contains(hook, "CASCADE_FISH_MODE") {
			t.Error("hook should check CASCADE_FISH_MODE for disable_arrow")
		}
	})

	t.Run("pipes to source", func(t *testing.T) {
		if !strings.Contains(hook, "| source") {
			t.Error("hook should pipe export output to source")
		}
	})
}

func TestFishExport(t *testing.T) {
	tests := []struct {
		name     string
		export   ShellExport
		contains []string
		excludes []string
	}{
		{
			name:     "empty export",
			export:   ShellExport{},
			contains: nil,
		},
		{
			name: "set single variable",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("FOO", "bar")
				return e
			}(),
			contains: []string{`set -gx FOO 'bar';`},
		},
		{
			name: "unset single variable",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Unset("FOO")
				return e
			}(),
			contains: []string{`set -e FOO;`},
		},
		{
			name: "set and unset multiple",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("PATH", "/usr/bin")
				e.Unset("OLD_VAR")
				e.Set("HOME", "/home/user")
				return e
			}(),
			contains: []string{
				`set -gx PATH '/usr/bin';`,
				`set -e OLD_VAR;`,
				`set -gx HOME '/home/user';`,
			},
		},
		{
			name: "value with single quotes",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("MSG", "it's a test")
				return e
			}(),
			contains: []string{`set -gx MSG 'it\'s a test';`},
		},
		{
			name: "value with backslash",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("PATH", `C:\Users\test`)
				return e
			}(),
			contains: []string{`set -gx PATH 'C:\\Users\\test';`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fish.Export(tt.export)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Export() = %q, should contain %q", got, want)
				}
			}
			for _, exclude := range tt.excludes {
				if strings.Contains(got, exclude) {
					t.Errorf("Export() = %q, should not contain %q", got, exclude)
				}
			}
		})
	}
}

func TestFishExportDeterministic(t *testing.T) {
	e := make(ShellExport)
	e.Set("Z_VAR", "last")
	e.Set("A_VAR", "first")
	e.Set("M_VAR", "middle")

	got := Fish.Export(e)

	// Check that A comes before M comes before Z
	aIdx := strings.Index(got, "A_VAR")
	mIdx := strings.Index(got, "M_VAR")
	zIdx := strings.Index(got, "Z_VAR")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("Export() output not sorted: A at %d, M at %d, Z at %d", aIdx, mIdx, zIdx)
	}
}

func TestFishDump(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains []string
	}{
		{
			name:     "empty env",
			env:      map[string]string{},
			contains: nil,
		},
		{
			name: "single variable",
			env: map[string]string{
				"FOO": "bar",
			},
			contains: []string{`set -gx FOO 'bar';`},
		},
		{
			name: "multiple variables",
			env: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
			},
			contains: []string{
				`set -gx PATH '/usr/bin';`,
				`set -gx HOME '/home/user';`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fish.Dump(tt.env)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Dump() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

func TestFishDumpDeterministic(t *testing.T) {
	env := map[string]string{
		"Z_VAR": "last",
		"A_VAR": "first",
		"M_VAR": "middle",
	}

	got := Fish.Dump(env)

	// Check that A comes before M comes before Z
	aIdx := strings.Index(got, "A_VAR")
	mIdx := strings.Index(got, "M_VAR")
	zIdx := strings.Index(got, "Z_VAR")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("Dump() output not sorted: A at %d, M at %d, Z at %d", aIdx, mIdx, zIdx)
	}
}

func TestFishEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "single quote",
			input: "it's",
			want:  `it\'s`,
		},
		{
			name:  "backslash",
			input: `path\to\file`,
			want:  `path\\to\\file`,
		},
		{
			name:  "multiple single quotes",
			input: "don't say 'hello'",
			want:  `don\'t say \'hello\'`,
		},
		{
			name:  "backslash before quote",
			input: `test\'value`,
			want:  `test\\\'value`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "unicode",
			input: "héllo wörld 日本語",
			want:  "héllo wörld 日本語",
		},
		{
			name:  "dollar sign unchanged",
			input: "$HOME/bin",
			want:  "$HOME/bin",
		},
		{
			name:  "double quotes unchanged",
			input: `say "hello"`,
			want:  `say "hello"`,
		},
		{
			name:  "newline unchanged",
			input: "line1\nline2",
			want:  "line1\nline2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FishEscape(tt.input)
			if got != tt.want {
				t.Errorf("FishEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetFish(t *testing.T) {
	got := Get("fish")
	if got == nil {
		t.Fatal("Get(\"fish\") returned nil")
	}
	if got.Name() != "fish" {
		t.Errorf("Get(\"fish\").Name() = %q, want %q", got.Name(), "fish")
	}
}

func TestSupportedIncludesFish(t *testing.T) {
	supported := Supported()

	found := false
	for _, s := range supported {
		if s == "fish" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Supported() should include 'fish'")
	}
}
