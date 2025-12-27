package cmd

import (
	"bytes"
	"testing"

	"github.com/unrss/cascade/internal/env"
)

func TestLogEnvDiff(t *testing.T) {
	tests := []struct {
		name      string
		diff      *env.EnvDiff
		unloading bool
		want      string
	}{
		{
			name:      "nil diff",
			diff:      nil,
			unloading: false,
			want:      "",
		},
		{
			name:      "empty diff",
			diff:      &env.EnvDiff{},
			unloading: false,
			want:      "",
		},
		{
			name: "added variables",
			diff: &env.EnvDiff{
				Prev: map[string]string{"FOO": "", "BAR": ""},
				Next: map[string]string{"FOO": "foo", "BAR": "bar"},
			},
			unloading: false,
			want:      "cascade export: +BAR +FOO\n",
		},
		{
			name: "removed variables",
			diff: &env.EnvDiff{
				Prev: map[string]string{"FOO": "foo", "BAR": "bar"},
				Next: map[string]string{"FOO": "", "BAR": ""},
			},
			unloading: true,
			want:      "cascade unloading: -BAR -FOO\n",
		},
		{
			name: "changed variables",
			diff: &env.EnvDiff{
				Prev: map[string]string{"FOO": "old"},
				Next: map[string]string{"FOO": "new"},
			},
			unloading: false,
			want:      "cascade export: ~FOO\n",
		},
		{
			name: "mixed changes",
			diff: &env.EnvDiff{
				Prev: map[string]string{"ADD": "", "DEL": "old", "CHG": "old"},
				Next: map[string]string{"ADD": "new", "DEL": "", "CHG": "new"},
			},
			unloading: false,
			want:      "cascade export: +ADD ~CHG -DEL\n",
		},
		{
			name: "sorted output",
			diff: &env.EnvDiff{
				Prev: map[string]string{"ZZZ": "", "AAA": "", "MMM": ""},
				Next: map[string]string{"ZZZ": "z", "AAA": "a", "MMM": "m"},
			},
			unloading: false,
			want:      "cascade export: +AAA +MMM +ZZZ\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logEnvDiff(&buf, tt.diff, tt.unloading)
			if got := buf.String(); got != tt.want {
				t.Errorf("logEnvDiff() = %q, want %q", got, tt.want)
			}
		})
	}
}
