package list

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func seed(t *testing.T, root string, ms ...memory.Memory) {
	t.Helper()
	for _, m := range ms {
		if err := memory.Add(context.Background(), root, m, &libagent.Outcome{}); err != nil {
			t.Fatal(err)
		}
	}
}

var (
	beta  = memory.Memory{Name: "beta", Description: "Second fact", Type: "project", Body: "b\n"}
	alpha = memory.Memory{Name: "alpha", Description: "First fact", Type: "user", Body: "a\n"}
)

func TestListPrintsIndexEntriesByName(t *testing.T) {
	root := identitytest.Repository(t)
	seed(t, root, beta, alpha)
	out := Run(context.Background(), root, Options{})
	want := Result{Memories: []Entry{{Name: "alpha", Description: "First fact", Type: "user"}, {Name: "beta", Description: "Second fact", Type: "project"}}}
	if out.Error != nil || out.Operation != "memory.list" || !reflect.DeepEqual(out.Result, want) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "- [alpha](alpha.md) — First fact\n- [beta](beta.md) — Second fact\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestListFiltersByType(t *testing.T) {
	root := identitytest.Repository(t)
	seed(t, root, beta, alpha)
	out := Run(context.Background(), root, Options{Type: "project"})
	if want := (Result{Memories: []Entry{{Name: "beta", Description: "Second fact", Type: "project"}}}); !reflect.DeepEqual(out.Result, want) {
		t.Fatalf("%+v", out)
	}
	out = Run(context.Background(), root, Options{Type: "note"})
	if out.Error == nil || out.Error.Code != "invalid_input" {
		t.Fatalf("%+v", out)
	}
}

func TestListWithoutMemoriesIsEmpty(t *testing.T) {
	root := identitytest.Repository(t)
	out := Run(context.Background(), root, Options{})
	if !reflect.DeepEqual(out.Result, Result{Memories: []Entry{}}) {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "No memories.\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestListOutsideRepositoryFails(t *testing.T) {
	out := Run(context.Background(), t.TempDir(), Options{})
	if out.Error == nil || out.Error.Phase != "state" {
		t.Fatalf("%+v", out)
	}
}
