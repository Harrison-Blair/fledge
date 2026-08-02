package processenv

import (
	"slices"
	"testing"
)

func TestWithoutNoColor(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want []string
	}{
		{
			name: "set",
			env:  []string{"TERM=xterm-256color", "NO_COLOR=1", "TOKEN=secret"},
			want: []string{"TERM=xterm-256color", "TOKEN=secret"},
		},
		{
			name: "empty",
			env:  []string{"NO_COLOR=", "COLORTERM=truecolor"},
			want: []string{"COLORTERM=truecolor"},
		},
		{
			name: "similar names",
			env:  []string{"MY_NO_COLOR=1", "NO_COLORFUL=1", "NO_COLOR_EXTRA=1"},
			want: []string{"MY_NO_COLOR=1", "NO_COLORFUL=1", "NO_COLOR_EXTRA=1"},
		},
		{
			name: "absent",
			env:  []string{"TERM=dumb", "TOKEN=secret"},
			want: []string{"TERM=dumb", "TOKEN=secret"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := slices.Clone(test.env)
			got := WithoutNoColor(test.env)
			if !slices.Equal(got, test.want) {
				t.Fatalf("filtered environment = %q, want %q", got, test.want)
			}
			if !slices.Equal(test.env, source) {
				t.Fatalf("source environment mutated: got %q, want %q", test.env, source)
			}
			if len(got) > 0 {
				got[0] = "CHANGED=1"
				if !slices.Equal(test.env, source) {
					t.Fatalf("result aliases source environment: got %q, want %q", test.env, source)
				}
			}
		})
	}
}

func TestManagedReplacesTempDirAndDoesNotMutateInput(t *testing.T) {
	source := []string{
		"TERM=xterm-256color", "TMPDIR=/inherited", "NO_COLOR=1",
		"FLEDGE_UNRELATED=unchanged", "TMPDIR=/duplicate",
	}
	original := slices.Clone(source)
	got := Managed(source, "/project/.fledge/tmp")
	want := []string{
		"TERM=xterm-256color", "FLEDGE_UNRELATED=unchanged", "TMPDIR=/project/.fledge/tmp",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("managed environment = %q, want %q", got, want)
	}
	if !slices.Equal(source, original) {
		t.Fatalf("source environment mutated: got %q, want %q", source, original)
	}
	got[0] = "CHANGED=1"
	if !slices.Equal(source, original) {
		t.Fatalf("result aliases source environment: got %q, want %q", source, original)
	}
}
