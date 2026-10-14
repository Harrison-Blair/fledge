package task

import (
	"context"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestRecordWithoutGraphFieldsLoads(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &cli.Outcome{})
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
	if err != nil || r.Parent != nil || r.After != nil || r.UnmetAtAssign != nil {
		t.Fatalf("%+v %v", r, err)
	}
	if got := Unmet(r, Index([]Record{r})); len(got) != 0 {
		t.Fatalf("%v", got)
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

func TestUnmetKeepsOrderAndTreatsCancelledAsSatisfied(t *testing.T) {
	rs := []Record{
		{ID: "00000001", Status: Verified},
		{ID: "00000002", Status: Cancelled},
		{ID: "00000003", Status: Completed},
		{ID: "00000004", Status: Created},
	}
	r := Record{ID: "0000000a", After: []string{"00000004", "00000001", "00000002", "00000003", "0000dead"}}
	if got, want := Unmet(r, Index(rs)), []string{"00000004", "00000003", "0000dead"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	for status, want := range map[string]bool{Created: false, Assigned: false, Completed: false, Verified: true, Cancelled: true} {
		if Satisfied(status) != want {
			t.Fatalf("%s", status)
		}
	}
}

func TestCheckLinks(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &cli.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, status := range []string{Created, Assigned, Completed, Verified, Cancelled} {
		ids[status], err = s.Create(Kind, func(id string) any {
			return Record{ID: id, Title: status, Brief: "b", Status: status, CreatedAt: *Now()}
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, status := range []string{Created, Assigned, Completed} {
		if err := CheckLinks(s, ids[status], []string{ids[Verified], ids[Cancelled]}); err != nil {
			t.Fatalf("%s parent: %v", status, err)
		}
	}
	if err := CheckLinks(s, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{Verified, Cancelled} {
		err := CheckLinks(s, ids[status], nil)
		if want := "task_invalid_state: task " + ids[status] + " is " + status + "; adding a subtask requires created or assigned or completed"; code(err) != "task_invalid_state" || err.Error() != want {
			t.Fatalf("%s parent: %v", status, err)
		}
	}
	// The parent is checked before prerequisites, and prerequisites in order.
	if err := CheckLinks(s, ids[Verified], []string{"0123abcd"}); code(err) != "task_invalid_state" {
		t.Fatalf("%v", err)
	}
	if err := CheckLinks(s, ids[Created], []string{ids[Created], "0123abcd", "0123abce"}); code(err) != "task_not_found" || err.Error() != "task_not_found: no task with id 0123abcd" {
		t.Fatalf("%v", err)
	}
	if err := CheckLinks(s, "0123abcd", nil); code(err) != "task_not_found" {
		t.Fatalf("%v", err)
	}
}

func TestLayersReturnsTheChainBottomFirst(t *testing.T) {
	p, other := "0000000a", "0000000b"
	layer := func(id, status string, after ...string) Record {
		return Record{ID: id, Parent: &p, Status: status, After: after}
	}
	rs := []Record{
		{ID: p, Shape: ShapeStack},
		layer("00000003", Created, "00000002", "0000000f"),
		layer("00000001", Verified, "0000000f"),
		layer("00000002", Cancelled, "00000001"),
		{ID: "0000000f", Status: Created},
		{ID: "00000004", Parent: &other, After: []string{"00000003"}},
	}
	got, err := Layers(p, rs)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []string{"00000001", "00000002", "00000003"}) {
		t.Fatalf("%v", ids)
	}
	if got, err := Layers(other, rs[:5]); err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestLayersRefusesABrokenChain(t *testing.T) {
	p := "0000000a"
	layer := func(id string, after ...string) Record {
		return Record{ID: id, Parent: &p, After: after}
	}
	const prefix = "stack layers must form one after chain: "
	for want, rs := range map[string][]Record{
		"layer 00000001 has more than one dependent layer: 00000002, 00000003": {layer("00000001"), layer("00000002", "00000001"), layer("00000003", "00000001")},
		"more than one layer has no prerequisite layer: 00000001, 00000002":    {layer("00000001"), layer("00000002"), layer("00000003", "00000002")},
		"layer 00000003 is after more than one layer: 00000001, 00000002":      {layer("00000001"), layer("00000002", "00000001"), layer("00000003", "00000001", "00000002")},
		"no layer is without a prerequisite layer":                             {layer("00000001", "00000002"), layer("00000002", "00000001")},
		"the chain from 00000001 does not include 00000003, 00000004":          {layer("00000001"), layer("00000002", "00000001"), layer("00000003", "00000004"), layer("00000004", "00000003")},
	} {
		if _, err := Layers(p, rs); err == nil || err.Error() != prefix+want {
			t.Fatalf("want %q, got %v", want, err)
		}
	}
}
