package species

import "testing"

// takenSet builds a taken func from a list of slugs.
func takenSet(slugs ...string) func(string) bool {
	set := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		set[s] = true
	}
	return func(slug string) bool { return set[slug] }
}

func TestPick(t *testing.T) {
	tests := []struct {
		name      string
		taken     []string
		requested string
		want      string
		wantErr   bool
	}{
		{
			name: "auto-pick first free",
			want: "emperor",
		},
		{
			name:  "auto-pick skips taken",
			taken: []string{"emperor", "king", "adelie"},
			want:  "chinstrap",
		},
		{
			name:    "pool exhausted",
			taken:   Slugs,
			wantErr: true,
		},
		{
			name:      "requested free slug",
			taken:     []string{"emperor"},
			requested: "gentoo",
			want:      "gentoo",
		},
		{
			name:      "requested taken slug",
			taken:     []string{"gentoo"},
			requested: "gentoo",
			wantErr:   true,
		},
		{
			name:      "requested unknown slug",
			requested: "puffin",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Pick(takenSet(tt.taken...), tt.requested)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Pick(%q) = %q, want error", tt.requested, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Pick(%q): %v", tt.requested, err)
			}
			if got != tt.want {
				t.Errorf("Pick(%q) = %q, want %q", tt.requested, got, tt.want)
			}
		})
	}
}

func TestSlugsAreUniqueAndComplete(t *testing.T) {
	if len(Slugs) != 18 {
		t.Errorf("len(Slugs) = %d, want 18", len(Slugs))
	}
	seen := make(map[string]bool, len(Slugs))
	for _, s := range Slugs {
		if seen[s] {
			t.Errorf("duplicate slug %q", s)
		}
		seen[s] = true
	}
}
