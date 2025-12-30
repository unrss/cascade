package env

import "testing"

func TestEnvDiff_EqualEffect(t *testing.T) {
	tests := []struct {
		name  string
		d     *EnvDiff
		other *EnvDiff
		want  bool
	}{
		{
			name:  "both nil",
			d:     nil,
			other: nil,
			want:  true,
		},
		{
			name: "first nil second not",
			d:    nil,
			other: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			want: false,
		},
		{
			name: "first not nil second nil",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: nil,
			want:  false,
		},
		{
			name: "same Next different Prev - should be equal",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": "old1"},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": "old2"},
				Next: map[string]string{"FOO": "bar"},
			},
			want: true,
		},
		{
			name: "same Next completely different Prev keys - should be equal",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": "x", "BAR": "y"},
				Next: map[string]string{"FOO": "bar", "BAZ": "qux"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"DIFFERENT": "values"},
				Next: map[string]string{"FOO": "bar", "BAZ": "qux"},
			},
			want: true,
		},
		{
			name: "different Next values",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "baz"},
			},
			want: false,
		},
		{
			name: "different Next lengths",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar", "BAZ": "qux"},
			},
			want: false,
		},
		{
			name: "both empty diffs",
			d: &EnvDiff{
				Prev: map[string]string{},
				Next: map[string]string{},
			},
			other: &EnvDiff{
				Prev: map[string]string{},
				Next: map[string]string{},
			},
			want: true,
		},
		{
			name: "different keys same length",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"BAZ": ""},
				Next: map[string]string{"BAZ": "bar"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.EqualEffect(tt.other); got != tt.want {
				t.Errorf("EnvDiff.EqualEffect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvDiff_Equal(t *testing.T) {
	tests := []struct {
		name  string
		d     *EnvDiff
		other *EnvDiff
		want  bool
	}{
		{
			name:  "both nil",
			d:     nil,
			other: nil,
			want:  true,
		},
		{
			name: "first nil second not",
			d:    nil,
			other: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			want: false,
		},
		{
			name: "first not nil second nil",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: nil,
			want:  false,
		},
		{
			name: "same content",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": "", "BAR": "old"},
				Next: map[string]string{"FOO": "bar", "BAR": "new"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": "", "BAR": "old"},
				Next: map[string]string{"FOO": "bar", "BAR": "new"},
			},
			want: true,
		},
		{
			name: "different Next values",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "baz"},
			},
			want: false,
		},
		{
			name: "different Prev values",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": "old1"},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": "old2"},
				Next: map[string]string{"FOO": "bar"},
			},
			want: false,
		},
		{
			name: "different Next lengths",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar", "BAZ": "qux"},
			},
			want: false,
		},
		{
			name: "different Prev lengths",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"FOO": "", "BAZ": "old"},
				Next: map[string]string{"FOO": "bar"},
			},
			want: false,
		},
		{
			name: "both empty diffs",
			d: &EnvDiff{
				Prev: map[string]string{},
				Next: map[string]string{},
			},
			other: &EnvDiff{
				Prev: map[string]string{},
				Next: map[string]string{},
			},
			want: true,
		},
		{
			name: "different keys same length",
			d: &EnvDiff{
				Prev: map[string]string{"FOO": ""},
				Next: map[string]string{"FOO": "bar"},
			},
			other: &EnvDiff{
				Prev: map[string]string{"BAZ": ""},
				Next: map[string]string{"BAZ": "bar"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Equal(tt.other); got != tt.want {
				t.Errorf("EnvDiff.Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}
