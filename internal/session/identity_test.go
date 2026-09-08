package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fledge/internal/herdr"
)

type fakePaneResolver struct {
	pane herdr.Pane
	err  error
}

func (f fakePaneResolver) CurrentPane(context.Context) (herdr.Pane, error) {
	return f.pane, f.err
}

func TestValidateAmbientPaneUsesAuthoritativeLiveLocation(t *testing.T) {
	env := map[string]string{"HERDR_SESSION": "session-a", "HERDR_PANE_ID": "pane-1", "HERDR_WORKSPACE_ID": "old-workspace", "HERDR_TAB_ID": "old-tab"}
	want := herdr.Pane{ID: "pane-1", WorkspaceID: "moved-workspace", TabID: "moved-tab"}
	name, pane, err := ValidateAmbientPane(context.Background(), func(key string) string { return env[key] }, []string{"session-a"}, func(name string) PaneResolver {
		if name != "session-a" {
			t.Fatalf("scoped name = %q", name)
		}
		return fakePaneResolver{pane: want}
	})
	if err != nil {
		t.Fatalf("ValidateAmbientPane() error = %v", err)
	}
	if name != "session-a" || pane != want {
		t.Fatalf("ValidateAmbientPane() = %q, %#v, want %q, %#v", name, pane, "session-a", want)
	}
}

func TestValidateAmbientPaneFailsClosed(t *testing.T) {
	wantResolve := errors.New("session unavailable")
	tests := []struct {
		name    string
		env     map[string]string
		allowed []string
		resolve PaneResolver
		want    string
	}{
		{name: "missing session", env: map[string]string{"HERDR_PANE_ID": "pane-1"}, allowed: []string{"session-a"}, want: "missing HERDR_SESSION"},
		{name: "missing pane", env: map[string]string{"HERDR_SESSION": "session-a"}, allowed: []string{"session-a"}, want: "missing HERDR_SESSION or HERDR_PANE_ID"},
		{name: "cross project", env: map[string]string{"HERDR_SESSION": "session-b", "HERDR_PANE_ID": "pane-1"}, allowed: []string{"session-a"}, want: "does not belong"},
		{name: "stopped session", env: map[string]string{"HERDR_SESSION": "session-a", "HERDR_PANE_ID": "pane-1"}, allowed: []string{"session-a"}, resolve: fakePaneResolver{err: wantResolve}, want: "session unavailable"},
		{name: "stale pane", env: map[string]string{"HERDR_SESSION": "session-a", "HERDR_PANE_ID": "pane-old"}, allowed: []string{"session-a"}, resolve: fakePaneResolver{pane: herdr.Pane{ID: "pane-live", WorkspaceID: "workspace", TabID: "tab"}}, want: "is stale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			_, _, err := ValidateAmbientPane(context.Background(), func(key string) string { return test.env[key] }, test.allowed, func(string) PaneResolver {
				called = true
				return test.resolve
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAmbientPane() error = %v, want containing %q", err, test.want)
			}
			if (test.name == "missing session" || test.name == "missing pane" || test.name == "cross project") && called {
				t.Fatal("resolver called before ambient identity rejection")
			}
		})
	}
}
