package repoconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

// write puts content at root's .fledge/config.toml and returns root.
func write(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".fledge"), 0o755)
	if err := os.WriteFile(filepath.Join(root, Path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRead(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, content, main, dev string
		devSet                   bool
	}{
		{"both branches", "schema_version = 1\n[branches]\nmain = \"trunk\"\ndev = \"integration\"\n", "trunk", "integration", true},
		{"dev only", "schema_version = 1\n[branches]\ndev = \"release/v2\"\n", "main", "release/v2", true},
		{"dev set to default", "schema_version = 1\n[branches]\ndev = \"dev\"\n", "main", "dev", true},
		{"main only", "schema_version = 1\n[branches]\nmain = \"trunk\"\n", "trunk", "dev", false},
		{"empty table", "schema_version = 1\n[branches]\n", "main", "dev", false},
		{"no table", "schema_version = 1\n", "main", "dev", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Read(ctx, write(t, tc.content))
			b := got.Branches
			if err != nil || b.Main != tc.main || b.Dev != tc.dev || b.DevSet != tc.devSet {
				t.Fatalf("got %+v, %v want %q %q %v", got, err, tc.main, tc.dev, tc.devSet)
			}
		})
	}
	t.Run("all keys", func(t *testing.T) {
		got, err := Read(ctx, write(t, "schema_version = 1\ndelivery_mode = \"pr\"\n[branches]\nmain = \"main\"\ndev = \"dev\"\n[inbox]\nack_timeout = \"5m\"\n"))
		want := Config{DeliveryMode: "pr"}
		want.Branches.Main, want.Branches.Dev, want.Branches.DevSet = "main", "dev", true
		want.Inbox.AckTimeout = 5 * time.Minute
		if err != nil || got != want {
			t.Fatalf("got %+v, %v want %+v", got, err, want)
		}
	})
	for _, mode := range []string{"local", "pr", "auto", "none"} {
		t.Run("delivery mode "+mode, func(t *testing.T) {
			got, err := Read(ctx, write(t, "schema_version = 1\ndelivery_mode = \""+mode+"\"\n"))
			if err != nil || got.DeliveryMode != mode {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
	for _, d := range []string{"10s", "1h", "60m"} {
		t.Run("ack timeout "+d, func(t *testing.T) {
			want, _ := time.ParseDuration(d)
			got, err := Read(ctx, write(t, "schema_version = 1\n[inbox]\nack_timeout = \""+d+"\"\n"))
			if err != nil || got.Inbox.AckTimeout != want {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
	var defaults Config
	defaults.Branches.Main, defaults.Branches.Dev = "main", "dev"
	t.Run("absent keys", func(t *testing.T) {
		for _, content := range []string{"schema_version = 1\n", "schema_version = 1\n[inbox]\n"} {
			if got, err := Read(ctx, write(t, content)); err != nil || got != defaults {
				t.Fatalf("%q: got %+v, %v", content, got, err)
			}
		}
	})
	t.Run("absent file", func(t *testing.T) {
		if got, err := Read(ctx, t.TempDir()); err != nil || got != defaults {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
}

// Every malformed file is an error naming the problem, never a default.
func TestReadRejectsInvalidFiles(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, content, want string }{
		{"no version", "[branches]\ndev = \"dev\"\n", "schema_version is required"},
		{"unsupported version", "schema_version = 2\n", "unsupported schema_version"},
		{"string version", "schema_version = \"1\"\n", "schema_version"},
		{"base branch", "schema_version = 1\nbase_branch = \"dev\"\n", `unknown key "base_branch"`},
		{"integer dev", "schema_version = 1\n[branches]\ndev = 5\n", "[branches] dev must be a string"},
		{"integer main", "schema_version = 1\n[branches]\nmain = 5\n", "[branches] main must be a string"},
		{"unknown branches key", "schema_version = 1\n[branches]\nstage = \"x\"\n", `unknown key "branches.stage"`},
		{"case branches key", "schema_version = 1\n[branches]\nDev = \"dev\"\n", `unknown key "branches.Dev"`},
		{"case branches", "schema_version = 1\n[Branches]\ndev = \"dev\"\n", `unknown key "Branches"`},
		{"branches not a table", "schema_version = 1\nbranches = \"dev\"\n", "branches must be a table"},
		{"branches array of tables", "schema_version = 1\n[[branches]]\ndev = \"dev\"\n", "branches must be a table"},
		{"unknown key", "schema_version = 1\nmerge_mode = \"x\"\n", `unknown key "merge_mode"`},
		{"case variant", "schema_version = 1\nDelivery_mode = \"pr\"\n", `unknown key "Delivery_mode"`},
		{"upper case variant", "schema_version = 1\nSCHEMA_VERSION = 1\n", `unknown key "SCHEMA_VERSION"`},
		{"unknown nested key", "schema_version = 1\n[extra]\nx = 1\n", `unknown key "extra"`},
		{"syntax", "schema_version = \n", "toml"},
		{"blank dev", "schema_version = 1\n[branches]\ndev = \" \"\n", "[branches] dev is blank"},
		{"blank main", "schema_version = 1\n[branches]\nmain = \"\"\n", "[branches] main is blank"},
		{"option dev", "schema_version = 1\n[branches]\ndev = \"-x\"\n", `[branches] dev "-x" starts with -`},
		{"dot dot dev", "schema_version = 1\n[branches]\ndev = \"a..b\"\n", `[branches] dev "a..b" is not a valid branch name`},
		{"dot dot main", "schema_version = 1\n[branches]\nmain = \"a..b\"\n", `[branches] main "a..b" is not a valid branch name`},
		{"previous branch", "schema_version = 1\n[branches]\ndev = \"@{-1}\"\n", "[branches] dev"},
		{"trailing newline", "schema_version = 1\n[branches]\ndev = \"dev\\n\"\n", `[branches] dev "dev\n" spans lines`},
		{"unknown delivery mode", "schema_version = 1\ndelivery_mode = \"push\"\n", "delivery_mode \"push\" is not local, pr, auto, or none"},
		{"pr-human delivery mode", "schema_version = 1\ndelivery_mode = \"pr-human\"\n", "delivery_mode \"pr-human\" is not local, pr, auto, or none"},
		{"empty delivery mode", "schema_version = 1\ndelivery_mode = \"\"\n", "delivery_mode \"\" is not local, pr, auto, or none"},
		{"upper delivery mode", "schema_version = 1\ndelivery_mode = \"PR\"\n", "delivery_mode \"PR\" is not local, pr, auto, or none"},
		{"integer delivery mode", "schema_version = 1\ndelivery_mode = 1\n", "delivery_mode must be a string: local, pr, auto, or none"},
		{"case delivery mode", "schema_version = 1\nDelivery_Mode = \"pr\"\n", `unknown key "Delivery_Mode"`},
		{"case inbox", "schema_version = 1\n[Inbox]\nack_timeout = \"5m\"\n", `unknown key "Inbox"`},
		{"case ack timeout", "schema_version = 1\n[inbox]\nAck_Timeout = \"5m\"\n", `unknown key "inbox.Ack_Timeout"`},
		{"unknown inbox key", "schema_version = 1\n[inbox]\nretries = 3\n", `unknown key "inbox.retries"`},
		{"inbox not a table", "schema_version = 1\ninbox = \"5m\"\n", "inbox must be a table"},
		{"inbox array of tables", "schema_version = 1\n[[inbox]]\nack_timeout = \"5m\"\n", "inbox must be a table"},
		{"integer ack timeout", "schema_version = 1\n[inbox]\nack_timeout = 300\n", "inbox.ack_timeout must be a duration string from 10s to 1h"},
		{"unparsable ack timeout", "schema_version = 1\n[inbox]\nack_timeout = \"5 minutes\"\n", "inbox.ack_timeout \"5 minutes\" is not a duration from 10s to 1h"},
		{"short ack timeout", "schema_version = 1\n[inbox]\nack_timeout = \"9s\"\n", "inbox.ack_timeout \"9s\" is not a duration from 10s to 1h"},
		{"long ack timeout", "schema_version = 1\n[inbox]\nack_timeout = \"1h0m1s\"\n", "inbox.ack_timeout \"1h0m1s\" is not a duration from 10s to 1h"},
		{"negative ack timeout", "schema_version = 1\n[inbox]\nack_timeout = \"-5m\"\n", "inbox.ack_timeout \"-5m\" is not a duration from 10s to 1h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Read(ctx, write(t, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), Path) {
				t.Fatalf("got %+v, %v want error containing %q", got, err, tc.want)
			}
		})
	}
}

// Only a regular file is opened: a symlink, directory, or FIFO is refused
// without reading, so a FIFO cannot block the caller.
func TestReadRefusesNonRegularFiles(t *testing.T) {
	ctx := context.Background()
	for name, create := range map[string]func(path string) error{
		"symlink": func(path string) error {
			target := filepath.Join(filepath.Dir(path), "real.toml")
			os.WriteFile(target, []byte("schema_version = 1\n"), 0o644)
			return os.Symlink(target, path)
		},
		"directory": func(path string) error { return os.Mkdir(path, 0o755) },
		"fifo":      func(path string) error { return syscall.Mkfifo(path, 0o644) },
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			os.Mkdir(filepath.Join(root, ".fledge"), 0o755)
			if err := create(filepath.Join(root, Path)); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := Read(ctx, root); done <- err }()
			select {
			case err := <-done:
				if !errors.Is(err, ErrNotRegular) {
					t.Fatalf("got %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Read blocked")
			}
		})
	}
}

func TestReadRefusesOversizedFile(t *testing.T) {
	root := write(t, "schema_version = 1\n#"+strings.Repeat("x", MaxSize)+"\n")
	if _, err := Read(context.Background(), root); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	// A file of exactly the limit is read.
	content := "schema_version = 1\n#"
	root = write(t, content+strings.Repeat("x", MaxSize-len(content)-1)+"\n")
	if _, err := Read(context.Background(), root); err != nil {
		t.Fatal(err)
	}
}

// @{-1} names the previously checked-out branch to git check-ref-format
// --branch; the refs/heads/ form rejects it even with checkout history.
func TestReadRejectsPreviousBranchSyntax(t *testing.T) {
	root := write(t, "schema_version = 1\n[branches]\ndev = \"@{-1}\"\n")
	gittest.Git(t, root, "init", "-q", "-b", "main")
	gittest.Commit(t, root)
	gittest.Git(t, root, "switch", "-qc", "other")
	gittest.Git(t, root, "switch", "-q", "main")
	if _, err := Read(context.Background(), root); err == nil || !strings.Contains(err.Error(), "[branches] dev") {
		t.Fatalf("got %v", err)
	}
}
