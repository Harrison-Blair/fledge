package add

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func options() Options {
	return Options{Name: "herdr-socket", Description: "Herdr commands need socket access", Type: "project", Body: "Use the socket.\n", BodySet: true}
}

func TestAddStoresMemoryAndRendersIt(t *testing.T) {
	root := identitytest.Repository(t)
	out := Run(context.Background(), root, options(), nil)
	if out.Error != nil || out.Operation != "memory.add" || out.Result != (memory.Memory{Name: "herdr-socket", Description: "Herdr commands need socket access", Type: "project", Body: "Use the socket.\n"}) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", "memories", "herdr-socket.md")); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Added memory herdr-socket: Herdr commands need socket access\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestAddReadsBodyFromStdin(t *testing.T) {
	root := identitytest.Repository(t)
	o := options()
	o.Body, o.BodySet, o.File, o.FileSet = "", false, "-", true
	out := Run(context.Background(), root, o, strings.NewReader("from stdin\n"))
	if m, ok := out.Result.(memory.Memory); out.Error != nil || !ok || m.Body != "from stdin\n" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestAddRejectsInvalidInputAndExistingName(t *testing.T) {
	root := identitytest.Repository(t)
	for name, mutate := range map[string]func(*Options){
		"no body":   func(o *Options) { o.BodySet = false },
		"both":      func(o *Options) { o.File, o.FileSet = "-", true },
		"bad name":  func(o *Options) { o.Name = "Bad Name" },
		"bad type":  func(o *Options) { o.Type = "note" },
		"multiline": func(o *Options) { o.Description = "a\nb" },
	} {
		o := options()
		mutate(&o)
		out := Run(context.Background(), root, o, strings.NewReader("x"))
		if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Phase != "validation" || out.ExitCode() != 2 {
			t.Errorf("%s: %+v", name, out.Error)
		}
	}
	Run(context.Background(), root, options(), nil)
	out := Run(context.Background(), root, options(), nil)
	if out.Error == nil || out.Error.Code != "memory_exists" || out.Status != "rejected" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	var b bytes.Buffer
	out.Write(&b, true, Render)
	var envelope struct{ Error struct{ Code string } }
	if err := json.Unmarshal(b.Bytes(), &envelope); err != nil || envelope.Error.Code != "memory_exists" {
		t.Fatal(b.String(), err)
	}
}
