package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdrtest"
)

func TestSessionsPruneRequiresSafeguardWithoutTTY(t *testing.T) {
	log := filepath.Join(t.TempDir(), "invocations")
	binary := fakePruneBinary(t, `{"sessions":[]}`, "", log)
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"sessions", "prune", "--herdr-bin", binary},
		strings.NewReader("yes\n"), &bytes.Buffer{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "--yes or --dry-run is required") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("Herdr was invoked before safeguard validation: %v", err)
	}
}

func TestSessionsPruneJSONDryRunFiltersAndSorts(t *testing.T) {
	sessions := `{"sessions":[
		{"name":"fledge-z","running":false},
		{"name":"other","running":false},
		{"name":"fledge-running","running":true},
		{"name":"fledge-default","running":false,"default":true},
		{"name":"","running":false},
		{"name":"fledge-a","running":false}
	]}`
	log := filepath.Join(t.TempDir(), "invocations")
	binary := fakePruneBinary(t, sessions, "", log)
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"sessions", "prune", "--dry-run", "--json", "--herdr-bin", binary,
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Candidates []string `json:"candidates"`
			Deleted    []string `json:"deleted"`
			DryRun     bool     `json:"dry_run"`
			Cancelled  bool     `json:"cancelled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || !envelope.Data.DryRun || envelope.Data.Cancelled ||
		strings.Join(envelope.Data.Candidates, ",") != "fledge-a,fledge-z" ||
		len(envelope.Data.Deleted) != 0 {
		t.Fatalf("unexpected result: %#v", envelope)
	}
	if strings.Contains(readFile(t, log), "session delete") {
		t.Fatalf("dry run deleted a session: %s", readFile(t, log))
	}
}

func TestSessionsPruneAllContinuesAfterFailures(t *testing.T) {
	sessions := `{"sessions":[
		{"name":"other","running":false},
		{"name":"fledge-bad","running":false},
		{"name":"fledge-good","running":false},
		{"name":"running","running":true}
	]}`
	log := filepath.Join(t.TempDir(), "invocations")
	binary := fakePruneBinary(t, sessions, "fledge-bad", log)
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"sessions", "prune", "--all", "--yes", "--json", "--herdr-bin", binary,
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	details, ok := envelope.Error.Details.(map[string]any)
	if envelope.Error.Code != "session_prune_failed" || !ok {
		t.Fatalf("unexpected error: %#v", envelope)
	}
	if got := stringSlice(details["deleted"]); strings.Join(got, ",") != "fledge-good,other" {
		t.Fatalf("successful deletions = %v, details=%#v", got, details)
	}
	invocations := readFile(t, log)
	for _, name := range []string{"fledge-bad", "fledge-good", "other"} {
		if !strings.Contains(invocations, "session delete "+name+" --json\n") {
			t.Fatalf("missing deletion attempt for %s: %s", name, invocations)
		}
	}
}

func TestSessionsPruneTTYConfirmsOnce(t *testing.T) {
	sessions := `{"sessions":[
		{"name":"fledge-z","running":false},
		{"name":"fledge-a","running":false}
	]}`
	for _, test := range []struct {
		name       string
		answer     string
		wantDelete bool
	}{
		{name: "yes", answer: "yes\n", wantDelete: true},
		{name: "cancel", answer: "no\n", wantDelete: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "invocations")
			binary := fakePruneBinary(t, sessions, "", log)
			var stdout, stderr bytes.Buffer
			env := &environment{
				in: strings.NewReader(test.answer), out: &stdout, errOut: &stderr,
				herdrBin: binary, stdinTTY: func() bool { return true },
			}
			root := newRoot(env)
			root.SetArgs([]string{"sessions", "prune", "--herdr-bin", binary})
			root.SetIn(env.in)
			root.SetOut(env.out)
			root.SetErr(env.errOut)
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute: %v stderr=%s", err, stderr.String())
			}
			output := stdout.String()
			if strings.Count(output, "Delete these sessions?") != 1 ||
				strings.Index(output, "fledge-a") > strings.Index(output, "fledge-z") {
				t.Fatalf("unexpected preview: %s", output)
			}
			deleted := strings.Contains(readFile(t, log), "session delete")
			if deleted != test.wantDelete {
				t.Fatalf("deleted=%t want=%t invocations=%s", deleted, test.wantDelete, readFile(t, log))
			}
			if !test.wantDelete && !strings.Contains(output, "Cancelled") {
				t.Fatalf("missing cancellation status: %s", output)
			}
		})
	}
}

func fakePruneBinary(t *testing.T, sessions, failSession, log string) string {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(sessions)); err != nil {
		t.Fatal(err)
	}
	deleteBody := `if [ "$3" = ` + strconv.Quote(failSession) + ` ]; then
  echo "injected delete failure" >&2
  exit 4
fi
printf '%s\n' '{"deleted":true}'
`
	return herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		InvocationLog: log,
		Sessions:      []herdrtest.SessionCase{{Payload: compact.String()}},
		Branches: []herdrtest.Branch{
			{Condition: `[ "$1" = "session" ] && [ "$2" = "delete" ]`, Body: deleteBody},
		},
	})
}

func stringSlice(value any) []string {
	raw, _ := value.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
