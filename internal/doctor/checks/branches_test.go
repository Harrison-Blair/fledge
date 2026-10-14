package checks_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
)

// gitRepo makes a repository with one commit on main and the given extra
// branches, and writes config to .fledge/config.toml when config is not empty.
func gitRepo(t *testing.T, config string, branches ...string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "init")
	for _, b := range branches {
		run("branch", b)
	}
	if config != "" {
		writeCache(t, filepath.Join(dir, ".fledge", "config.toml"), config)
	}
	return dir
}

func cwdEnv(dir string) checks.Environment {
	e := healthyEnv()
	e.Getwd = func() (string, error) { return dir, nil }
	return e
}

func TestBranchesDevPresent(t *testing.T) {
	c := checks.Branches(context.Background(), cwdEnv(gitRepo(t, "", "dev")))
	if c.Name != "branches" || c.Status != report.OK {
		t.Fatalf("branches %+v", c)
	}
	want := report.BranchesData{Main: "main", Dev: "dev", Ref: "refs/heads/dev"}
	if c.Data != want {
		t.Fatalf("data = %#v, want %#v", c.Data, want)
	}
}

func TestBranchesDevOnOrigin(t *testing.T) {
	dir := gitRepo(t, "")
	if out, err := exec.Command("git", "-C", dir, "update-ref", "refs/remotes/origin/dev", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("update-ref: %v\n%s", err, out)
	}
	c := checks.Branches(context.Background(), cwdEnv(dir))
	if c.Status != report.OK || c.Data.(report.BranchesData).Ref != "refs/remotes/origin/dev" {
		t.Fatalf("branches %+v", c)
	}
}

func TestBranchesDevMissingWarns(t *testing.T) {
	for name, tc := range map[string]struct{ config, dev string }{
		"default":    {"", "dev"},
		"configured": {"schema_version = 1\n[branches]\ndev = \"trunk\"\n", "trunk"},
	} {
		t.Run(name, func(t *testing.T) {
			c := checks.Branches(context.Background(), cwdEnv(gitRepo(t, tc.config)))
			if c.Status != report.Warn {
				t.Fatalf("branches %+v", c)
			}
			for _, want := range []string{"git branch " + tc.dev + " main", "git push -u origin " + tc.dev} {
				if !strings.Contains(c.Detail, want) {
					t.Fatalf("detail %q does not contain %q", c.Detail, want)
				}
			}
		})
	}
}

func TestBranchesOutsideRepository(t *testing.T) {
	c := checks.Branches(context.Background(), cwdEnv(t.TempDir()))
	if c.Status != report.OK || c.Detail != "not in a repository" {
		t.Fatalf("branches %+v", c)
	}
}

func TestBranchesInvalidConfigWarns(t *testing.T) {
	c := checks.Branches(context.Background(), cwdEnv(gitRepo(t, "schema_version = 1\n[branches]\ndev = 5\n")))
	if c.Status != report.Warn || !strings.Contains(c.Detail, ".fledge/config.toml") {
		t.Fatalf("branches %+v", c)
	}
}

func TestBranchesGetwdErrorWarns(t *testing.T) {
	e := healthyEnv()
	e.Getwd = func() (string, error) { return "", os.ErrPermission }
	if c := checks.Branches(context.Background(), e); c.Status != report.Warn {
		t.Fatalf("branches %+v", c)
	}
}
