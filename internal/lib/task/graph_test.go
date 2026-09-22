package task

import (
	"context"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestRecordWithoutGraphFieldsLoads(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(Kind, func(id string) any {
		return map[string]any{"id": id, "title": "old", "brief": "b", "status": Created, "forced": false, "created_at": "2026-01-01T00:00:00Z"}
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Get(s, id)
	if err != nil || r.Parent != nil {
		t.Fatalf("%+v %v", r, err)
	}
	if p := ChildProgress(id, []Record{r}); p != nil {
		t.Fatalf("%+v", p)
	}
}

func TestChildProgressCountsDirectChildren(t *testing.T) {
	p, other := "0000000a", "0000000b"
	rs := []Record{
		{ID: p, Status: Assigned},
		{ID: "00000001", Parent: &p, Status: Verified},
		{ID: "00000002", Parent: &p, Status: Verified},
		{ID: "00000003", Parent: &p, Status: Completed},
		{ID: "00000004", Parent: &p, Status: Cancelled},
		{ID: "00000005", Parent: &other, Status: Verified},
	}
	rs = append(rs, Record{ID: "00000006", Parent: tptr("00000003"), Status: Created})
	got := ChildProgress(p, rs)
	if want := (&Progress{Verified: 2, Total: 3, Cancelled: 1}); !reflect.DeepEqual(got, want) || got.String() != "2/3 verified, 1 cancelled" {
		t.Fatalf("%+v %q", got, got.String())
	}
	if got := ChildProgress(other, rs); got.String() != "1/1 verified" {
		t.Fatalf("%q", got.String())
	}
	if got := ChildProgress("00000005", rs); got != nil {
		t.Fatalf("%+v", got)
	}
}

func tptr(s string) *string { return &s }
