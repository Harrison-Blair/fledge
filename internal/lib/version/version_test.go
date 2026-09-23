package version

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	for _, tc := range []struct{ release, module, want string }{
		{"v1.2.3", "v0.0.1+dirty", "v1.2.3"},
		{"", "v1.2.3", "v1.2.3"},
		{"", "v0.0.4-0.20260920031334-9d33d8ff0171", "v0.0.4-0.20260920031334-9d33d8ff0171"},
		{"", "v1.2.3+dirty", "v1.2.3+dirty"},
		{"", "(devel)", "dev"},
		{"", "", "dev"},
	} {
		info := &debug.BuildInfo{Main: debug.Module{Version: tc.module}}
		if got := resolve(tc.release, info); got != tc.want {
			t.Errorf("resolve(%q, %q) = %q, want %q", tc.release, tc.module, got, tc.want)
		}
	}
	if got := resolve("", nil); got != "dev" {
		t.Errorf("resolve without metadata = %q", got)
	}
}

func TestVersionUsesBuildMetadata(t *testing.T) {
	want := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		want = info.Main.Version
	}
	if got := Version(); got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
	t.Chdir(t.TempDir())
	if got := Version(); got != want {
		t.Fatalf("Version() outside repository = %q, want %q", got, want)
	}
}
