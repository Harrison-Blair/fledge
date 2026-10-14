package task

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestRecordWithoutLandingFieldsLoadsAsNone(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &cli.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(Kind, func(id string) any {
		return map[string]any{"id": id, "title": "old", "brief": "b", "status": Verified, "forced": false, "created_at": "2026-01-01T00:00:00Z"}
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Get(s, id)
	if err != nil || r.DeliveryMode != "none" {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Branch != nil || r.BaseBranch != nil || r.VerifiedCommit != nil || r.LandedAt != nil || r.LandedBy != nil ||
		r.MergeCommit != nil || r.TargetBranch != nil || r.PRURL != nil || r.LandStep != nil {
		t.Fatalf("%+v", r)
	}
	// An Update round trip writes the default back.
	r, err = Update(s, id, func(*Record) error { return nil })
	if err != nil || r.DeliveryMode != "none" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestRecordKeepsStoredDeliveryMode(t *testing.T) {
	var r Record
	if err := json.Unmarshal([]byte(`{"id":"0000000a","delivery_mode":"pr","branch":"b","verified_commit":"c"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.DeliveryMode != "pr" || *r.Branch != "b" || *r.VerifiedCommit != "c" || r.ID != "0000000a" {
		t.Fatalf("%+v", r)
	}
	b, err := json.Marshal(Record{DeliveryMode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"delivery_mode", "branch", "base_branch", "verified_commit", "verified_patch_id", "carried_from", "landed_at", "landed_by", "merge_commit", "target_branch", "pr_url", "land_step", "merge_request"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing %s in %s", k, b)
		}
	}
}

func TestLandedCountsAsVerified(t *testing.T) {
	if !Satisfied(Landed) {
		t.Fatal("landed is not satisfied")
	}
	p := "0000000a"
	rs := []Record{
		{ID: "00000001", Parent: &p, Status: Landed},
		{ID: "00000002", Parent: &p, Status: Verified},
		{ID: "00000003", Parent: &p, Status: Completed},
	}
	if got := ChildProgress(p, rs); got.String() != "2/3 verified" {
		t.Fatalf("%q", got.String())
	}
	if Statuses[len(Statuses)-1] != Landed {
		t.Fatalf("%v", Statuses)
	}
}
