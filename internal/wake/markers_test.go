package wake

import (
	"os"
	"reflect"
	"testing"
)

func fullMarkers() Markers {
	return Markers{
		Version:         markersVersion,
		StatusSeen:      map[string]StatusSeen{"alice": {Size: 128, MtimeUnix: 1754300000, Offset: 96}},
		EventEscalated:  map[string]bool{"%3": true},
		DoneGrace:       map[string]int64{"bob": 1754300090},
		KnownAgents:     []string{"alice", "bob"},
		LastWakeUnix:    1754300100,
		HeartbeatStreak: 3,
	}
}

func TestDecodeMarkersToleratesUnusableContents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		contents string
		want     Markers
	}{
		{name: "empty file", contents: "", want: emptyMarkers()},
		{name: "truncated JSON", contents: `{"version":1,"known_agents":["ali`, want: emptyMarkers()},
		{name: "not an object", contents: `["alice"]`, want: emptyMarkers()},
		{name: "wrong type", contents: `{"version":1,"heartbeat_streak":"three"}`, want: emptyMarkers()},
		{name: "missing version", contents: `{"heartbeat_streak":3}`, want: emptyMarkers()},
		{name: "future version", contents: `{"version":99,"heartbeat_streak":3}`, want: emptyMarkers()},
		{
			name:     "valid",
			contents: `{"version":1,"known_agents":["alice"],"heartbeat_streak":3}`,
			want:     Markers{Version: markersVersion, KnownAgents: []string{"alice"}, HeartbeatStreak: 3},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := decodeMarkers([]byte(test.contents)); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("decodeMarkers(%q) = %+v, want %+v", test.contents, got, test.want)
			}
		})
	}
}

func TestLoadMarkersReturnsEmptyMarkersWhenAbsentOrCorrupt(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)

	markers, err := ledger.LoadMarkers()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(markers, emptyMarkers()) {
		t.Fatalf("LoadMarkers() with no file = %+v, want %+v", markers, emptyMarkers())
	}

	if err := os.WriteFile(ledger.markersPath(), []byte(`{"version":1,"known_ag`), 0o600); err != nil {
		t.Fatal(err)
	}
	markers, err = ledger.LoadMarkers()
	if err != nil {
		t.Fatalf("LoadMarkers() with a corrupt file returned an error: %v", err)
	}
	if !reflect.DeepEqual(markers, emptyMarkers()) {
		t.Fatalf("LoadMarkers() with a corrupt file = %+v, want %+v", markers, emptyMarkers())
	}
}

func TestSaveAndLoadMarkersRoundTrip(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	want := fullMarkers()
	if err := ledger.SaveMarkers(want); err != nil {
		t.Fatal(err)
	}

	got, err := New(root, testSession).LoadMarkers()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadMarkers() = %+v, want %+v", got, want)
	}
}

func TestSaveMarkersStampsTheCurrentVersion(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	if err := ledger.SaveMarkers(Markers{HeartbeatStreak: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := ledger.LoadMarkers()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != markersVersion || got.HeartbeatStreak != 2 {
		t.Fatalf("LoadMarkers() = %+v, want version %d and streak 2", got, markersVersion)
	}
}

func TestSaveMarkersReplacesTheFileAtomically(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	if err := ledger.SaveMarkers(Markers{HeartbeatStreak: 1}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(ledger.markersPath())
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(ledger.markersPath())
	if err != nil {
		t.Fatal(err)
	}

	// An open reader of the previous markers keeps reading them: the replacement
	// is a rename, never an in-place truncate-and-rewrite.
	reader, err := os.Open(ledger.markersPath())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := ledger.SaveMarkers(fullMarkers()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(ledger.markersPath())
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("SaveMarkers rewrote the markers file in place")
	}

	held := make([]byte, len(original))
	if _, err := reader.Read(held); err != nil {
		t.Fatal(err)
	}
	if string(held) != string(original) {
		t.Fatalf("previously opened markers changed: %q, want %q", held, original)
	}

	names, err := os.ReadDir(ledger.watchPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		switch name.Name() {
		case markersFilename, logFilename, lockFilename:
		default:
			t.Fatalf("SaveMarkers left %q behind", name.Name())
		}
	}
}
