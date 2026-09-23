package release

import "testing"

func TestParseVersion_Rejects(t *testing.T) {
	bad := []string{
		"", "dev", "v1", "v1.2", "1.2.3", "v1.2.3.4", "v1.2.3-rc1",
		"v01.2.3", "v1.02.3", "v1.2.x", "va.b.c", " v1.2.3", "v-1.2.3", "v1.2.3+meta",
	}
	for _, s := range bad {
		if _, err := ParseVersion(s); err == nil {
			t.Errorf("ParseVersion(%q) = nil error, want an error", s)
		}
	}
}

func TestParseVersion_Accepts(t *testing.T) {
	v, err := ParseVersion("v0.10.3")
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	if v.Major != 0 || v.Minor != 10 || v.Patch != 3 {
		t.Fatalf("got %+v, want {0 10 3}", v)
	}
	if got := v.String(); got != "v0.10.3" {
		t.Fatalf("String() = %q, want v0.10.3", got)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b    string
		want    int
		wantErr bool
	}{
		{a: "v1.2.3", b: "v1.2.3", want: 0},
		{a: "v1.2.4", b: "v1.2.3", want: 1},
		{a: "v1.2.3", b: "v1.2.4", want: -1},
		{a: "v0.9.0", b: "v0.10.0", want: -1},
		{a: "v0.10.0", b: "v0.9.0", want: 1},
		{a: "v2.0.0", b: "v1.99.99", want: 1},
		{a: "v1.0.0", b: "v1.1.0", want: -1},
		{a: "dev", b: "v1.0.0", wantErr: true},
		{a: "v1.0.0", b: "nope", wantErr: true},
	}
	for _, tc := range tests {
		got, err := CompareVersions(tc.a, tc.b)
		if tc.wantErr {
			if err == nil {
				t.Errorf("CompareVersions(%q, %q) = %d, nil; want an error", tc.a, tc.b, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("CompareVersions(%q, %q): %v", tc.a, tc.b, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
