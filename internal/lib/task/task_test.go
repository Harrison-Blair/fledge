package task

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func code(err error) string {
	var remote *herdr.Error
	if errors.As(err, &remote) {
		return remote.Code
	}
	var input *libagent.InputError
	if errors.As(err, &input) {
		return "invalid_input"
	}
	return ""
}

func TestGetBeforeAnyStateIsNotFound(t *testing.T) {
	s, err := Existing(context.Background(), identitytest.Repository(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Get(s, "0123abcd"); code(err) != "task_not_found" {
		t.Fatalf("%v", err)
	}
	if _, err := Get(s, "nothex!!"); code(err) != "invalid_input" {
		t.Fatalf("%v", err)
	}
}

func TestUpdateStoresMutationAndRejectsOnError(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(Kind, func(id string) any { return Record{ID: id, Title: "t", Status: Created} })
	if err != nil {
		t.Fatal(err)
	}
	got, err := Update(s, id, func(r *Record) error { return Require(r, "assign", Created) })
	if err != nil || got.Status != Created {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := Update(s, id, func(r *Record) error { r.Title = "changed"; return Require(r, "complete", Assigned) }); code(err) != "task_invalid_state" {
		t.Fatalf("%v", err)
	}
	stored, err := Get(s, id)
	if err != nil || !reflect.DeepEqual(stored, Record{ID: id, Title: "t", Status: Created}) {
		t.Fatalf("%+v %v", stored, err)
	}
	if _, err := Update(s, "0123abcd", func(*Record) error { return nil }); code(err) != "task_not_found" {
		t.Fatalf("%v", err)
	}
}

func TestListIsOldestFirst(t *testing.T) {
	cwd := identitytest.Repository(t)
	if rs, err := List(nil); err != nil || rs == nil || len(rs) != 0 {
		t.Fatalf("%v %v", rs, err)
	}
	s, err := identity.OpenStore(context.Background(), cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []string{"2026-01-03T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"} {
		if _, err := s.Create(Kind, func(id string) any { return Record{ID: id, Title: at, CreatedAt: at} }); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := List(s)
	var titles []string
	for _, r := range rs {
		titles = append(titles, r.Title)
	}
	if want := []string{"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z"}; err != nil || !reflect.DeepEqual(titles, want) {
		t.Fatalf("%v %v", titles, err)
	}
}

func titles(t *testing.T, s *state.Store) []string {
	t.Helper()
	rs, err := List(s)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rs {
		out = append(out, r.Title)
	}
	return out
}

func TestListOrdersBurstCreatesByCreation(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1", "2", "3", "4", "5", "6"}
	for _, title := range want {
		if _, err := s.Create(Kind, func(id string) any { return Record{ID: id, Title: title, CreatedAt: *Now()} }); err != nil {
			t.Fatal(err)
		}
	}
	if got := titles(t, s); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListOrdersMixedPrecisionTimesByInstant(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	// Whole-second records predate nanosecond timestamps; a string compare
	// would put "00.5Z" before "00Z".
	for _, at := range []string{"2026-01-01T00:00:00.5Z", "2026-01-01T00:00:01Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00.000000001Z"} {
		if _, err := s.Create(Kind, func(id string) any { return Record{ID: id, Title: at, CreatedAt: at} }); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"2026-01-01T00:00:00Z", "2026-01-01T00:00:00.000000001Z", "2026-01-01T00:00:00.5Z", "2026-01-01T00:00:01Z"}
	if got := titles(t, s); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListBreaksTimeTiesByID(t *testing.T) {
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := s.Create(Kind, func(id string) any { return Record{ID: id, Title: id, CreatedAt: "2026-01-01T00:00:00Z"} }); err != nil {
			t.Fatal(err)
		}
	}
	got := titles(t, s)
	if want := slices.Sorted(slices.Values(got)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want ids ascending", got)
	}
}
