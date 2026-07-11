package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newUpdateTestServer starts an httptest server serving a canned GitHub
// releases/latest JSON body, and returns it. Caller must Close() it.
func newUpdateTestServer(t *testing.T, tag, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name": %q, "body": %q, "assets": []}`, tag, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withUpdateBaseURL points the update command's GitHub API base URL at the
// given server for the duration of the test, restoring it afterward.
func withUpdateBaseURL(t *testing.T, url string) {
	t.Helper()
	prev := updateAPIBaseURL
	updateAPIBaseURL = url
	t.Cleanup(func() { updateAPIBaseURL = prev })
}

func TestUpdate_AlreadyUpToDate(t *testing.T) {
	srv := newUpdateTestServer(t, "v"+binaryVersion, "some notes")
	withUpdateBaseURL(t, srv.URL)

	var out strings.Builder
	code := runUpdateWith(nil, strings.NewReader(""), &out)

	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("output = %q, want up-to-date message", out.String())
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("output = %q, should not contain a prompt", out.String())
	}
}

func TestUpdate_NewerAvailable_PromptsAndShowsNotes(t *testing.T) {
	srv := newUpdateTestServer(t, "v99.99.99", "shiny new release notes")
	withUpdateBaseURL(t, srv.URL)

	var out strings.Builder
	code := runUpdateWith(nil, strings.NewReader("\n"), &out)

	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code)
	}
	got := out.String()
	if !strings.Contains(got, binaryVersion) {
		t.Errorf("output = %q, want current version %q", got, binaryVersion)
	}
	if !strings.Contains(got, "99.99.99") {
		t.Errorf("output = %q, want latest version 99.99.99", got)
	}
	if !strings.Contains(got, "shiny new release notes") {
		t.Errorf("output = %q, want release notes", got)
	}
	if !strings.Contains(got, "[y/N]") {
		t.Errorf("output = %q, want a confirm prompt", got)
	}
}

func TestUpdate_ConfirmYes(t *testing.T) {
	srv := newUpdateTestServer(t, "v99.99.99", "notes")
	withUpdateBaseURL(t, srv.URL)

	var out strings.Builder
	code := runUpdateWith(nil, strings.NewReader("y\n"), &out)

	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code)
	}
	if !strings.Contains(out.String(), "update mechanics not yet implemented") {
		t.Errorf("output = %q, want placeholder confirm-path action", out.String())
	}
}

func TestUpdate_ConfirmDefaultDeny(t *testing.T) {
	srv := newUpdateTestServer(t, "v99.99.99", "notes")
	withUpdateBaseURL(t, srv.URL)

	var out strings.Builder
	code := runUpdateWith(nil, strings.NewReader("\n"), &out)

	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code)
	}
	if strings.Contains(out.String(), "update mechanics not yet implemented") {
		t.Errorf("output = %q, should not have taken confirm-path action on default deny", out.String())
	}
}

func TestUpdate_YesFlagSkipsPrompt(t *testing.T) {
	srv := newUpdateTestServer(t, "v99.99.99", "notes")
	withUpdateBaseURL(t, srv.URL)

	var out strings.Builder
	code := runUpdateWith([]string{"--yes"}, strings.NewReader(""), &out)

	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code)
	}
	got := out.String()
	if !strings.Contains(got, "update mechanics not yet implemented") {
		t.Errorf("output = %q, want placeholder confirm-path action", got)
	}
	if strings.Contains(got, "[y/N]") {
		t.Errorf("output = %q, --yes should skip the prompt", got)
	}
}

func TestUpdate_JSONFlagIsDryRun(t *testing.T) {
	// Case 1: up to date.
	srv := newUpdateTestServer(t, "v"+binaryVersion, "notes here")
	withUpdateBaseURL(t, srv.URL)

	var out strings.Builder
	code := runUpdateWith([]string{"--json"}, strings.NewReader(""), &out)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code)
	}
	var got updateJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, out.String())
	}
	if got.Current != binaryVersion || got.Latest != binaryVersion || !got.UpToDate || got.Notes != "notes here" {
		t.Errorf("got %+v, want current=%s latest=%s upToDate=true notes=%q", got, binaryVersion, binaryVersion, "notes here")
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("output = %q, --json must never prompt", out.String())
	}

	// Case 2: newer available.
	srv2 := newUpdateTestServer(t, "v99.99.99", "newer notes")
	withUpdateBaseURL(t, srv2.URL)

	var out2 strings.Builder
	code2 := runUpdateWith([]string{"--json"}, strings.NewReader(""), &out2)
	if code2 != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK", code2)
	}
	var got2 updateJSON
	if err := json.Unmarshal([]byte(out2.String()), &got2); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, out2.String())
	}
	if got2.Current != binaryVersion || got2.Latest != "99.99.99" || got2.UpToDate || got2.Notes != "newer notes" {
		t.Errorf("got %+v, want current=%s latest=99.99.99 upToDate=false notes=%q", got2, binaryVersion, "newer notes")
	}
	if strings.Contains(out2.String(), "[y/N]") {
		t.Errorf("output = %q, --json must never prompt", out2.String())
	}
}
