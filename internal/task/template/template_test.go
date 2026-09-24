package template

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/proposal"
)

func TestTemplatePrintsSkeletons(t *testing.T) {
	// Run takes no client or directory, so it cannot reach Herdr or the store.
	for _, tc := range []struct {
		options Options
		kind    string
		text    string
	}{
		{Options{}, "brief", brief.Skeleton()},
		{Options{Proposal: true}, "proposal", proposal.Skeleton()},
	} {
		out := Run(tc.options)
		if out.Error != nil || out.Operation != "task.template" || out.Status != "success" || !reflect.DeepEqual(out.Result, Result{Kind: tc.kind, Text: tc.text}) {
			t.Fatalf("%+v", out)
		}
		var b bytes.Buffer
		if err := out.Write(&b, false, Render); err != nil {
			t.Fatal(err)
		}
		if b.String() != tc.text {
			t.Fatalf("%q\nwant %q", b.String(), tc.text)
		}
		b.Reset()
		if err := out.Write(&b, true, Render); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Operation string            `json:"operation"`
			Status    string            `json:"status"`
			Result    map[string]string `json:"result"`
			Effects   []libagent.Effect `json:"effects"`
			Error     *libagent.Failure `json:"error"`
		}
		if err := json.Unmarshal(b.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Operation != "task.template" || got.Status != "success" || got.Error != nil || got.Effects == nil ||
			!reflect.DeepEqual(got.Result, map[string]string{"kind": tc.kind, "text": tc.text}) {
			t.Fatalf("%s", b.String())
		}
	}
}
