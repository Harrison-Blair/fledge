package buildinfo

import "testing"

func TestEmbeddedVersion(t *testing.T) {
	if Version() != "v0.0.1" {
		t.Fatalf("version = %q", Version())
	}
	info := Current()
	if info.Version != Version() || info.GoVersion == "" {
		t.Fatalf("info = %#v", info)
	}
}
