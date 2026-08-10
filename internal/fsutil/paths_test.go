package fsutil

import (
	"path/filepath"
	"testing"
)

func TestPathsNestSessionLogsBeneathState(t *testing.T) {
	t.Parallel()

	root := filepath.Join("home", "project")
	if got, want := Root(root), filepath.Join(root, ".fledge"); got != want {
		t.Errorf("Root() = %q, want %q", got, want)
	}
	if got, want := Logs(root), filepath.Join(root, ".fledge", "logs"); got != want {
		t.Errorf("Logs() = %q, want %q", got, want)
	}
	if got, want := Session(root, "fledge-demo-0a1b2c3d"), filepath.Join(root, ".fledge", "logs", "fledge-demo-0a1b2c3d"); got != want {
		t.Errorf("Session() = %q, want %q", got, want)
	}
	if got, want := Temp(root), filepath.Join(root, ".fledge", "tmp"); got != want {
		t.Errorf("Temp() = %q, want %q", got, want)
	}
	if got, want := TempSession(root, "fledge-demo-0a1b2c3d"), filepath.Join(root, ".fledge", "tmp", "fledge-demo-0a1b2c3d"); got != want {
		t.Errorf("TempSession() = %q, want %q", got, want)
	}
}

func TestValidSessionDirName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: "current grammar", give: "fledge-demo-0a1b2c3d", want: true},
		{name: "multi segment slug", give: "fledge-my-demo-project-0a1b2c3d", want: true},
		{name: "legacy grammar", give: "fledge-00000000000000000000000000000000", want: true},
		{name: "blank", give: ""},
		{name: "whitespace", give: "   "},
		{name: "current directory", give: "."},
		{name: "parent directory", give: ".."},
		{name: "path separator", give: "fledge-demo-0a1b2c3d/child"},
		{name: "traversal", give: "../fledge-demo-0a1b2c3d"},
		{name: "windows separator", give: `fledge-demo-0a1b2c3d\child`},
		{name: "missing prefix", give: "demo-0a1b2c3d"},
		{name: "uppercase slug", give: "fledge-Demo-0a1b2c3d"},
		{name: "short suffix", give: "fledge-demo-0a1b2c3"},
		{name: "non hex suffix", give: "fledge-demo-0a1b2c3g"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidSessionDirName(test.give); got != test.want {
				t.Errorf("ValidSessionDirName(%q) = %v, want %v", test.give, got, test.want)
			}
		})
	}
}
