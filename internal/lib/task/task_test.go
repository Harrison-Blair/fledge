package task

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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
