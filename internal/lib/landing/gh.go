package landing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

// GH runs the GitHub CLI in the checkout at Dir. Every value is passed as
// --flag=value, so none can be read as another option.
type GH struct{ Dir string }

// PR is an open pull request as gh pr list reports it.
type PR struct {
	Number     int    `json:"number"`
	URL        string `json:"url"`
	HeadRefOid string `json:"headRefOid"`
}

// Checks is the state of a pull request that decides whether it can merge.
type Checks struct {
	IsDraft    bool
	Mergeable  string
	HeadRefOid string
	Rollup     []Check
}

// Check is one entry of a status check rollup: a check run, with Status and
// Conclusion, or a status context, with State.
type Check struct {
	Typename, Status, Conclusion, State string
}

// Merged is the state, merge commit, and head of a pull request.
type Merged struct {
	State, MergeCommit, HeadRefOid string
}

// run runs gh with args and returns its stdout, or a gh_failed error with
// its stderr.
func (g GH) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = g.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, &cli.Error{Code: "land_gh_failed", Message: fmt.Sprintf("gh %s failed: %v: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))}
	}
	return b, nil
}

// List returns the open pull requests from head into base.
func (g GH) List(ctx context.Context, head, base string) ([]PR, error) {
	b, err := g.run(ctx, "pr", "list", "--head="+head, "--base="+base, "--state=open", "--json=number,url,headRefOid,isDraft,mergeable")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Number     *int    `json:"number"`
		URL        *string `json:"url"`
		HeadRefOid *string `json:"headRefOid"`
		IsDraft    *bool   `json:"isDraft"`
		Mergeable  *string `json:"mergeable"`
	}
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return nil, invalidOutput("pr list", err)
	}
	prs := []PR{}
	for _, p := range raw {
		if p.Number == nil || p.URL == nil || p.HeadRefOid == nil || p.IsDraft == nil || p.Mergeable == nil {
			return nil, invalidOutput("pr list", errors.New("a pull request is missing a field"))
		}
		prs = append(prs, PR{Number: *p.Number, URL: *p.URL, HeadRefOid: *p.HeadRefOid})
	}
	return prs, nil
}

// Create opens a pull request from head into base with title and the body
// in bodyFile, and returns the URL gh prints for it.
func (g GH) Create(ctx context.Context, head, base, title, bodyFile string) (string, error) {
	b, err := g.run(ctx, "pr", "create", "--base="+base, "--head="+head, "--title="+title, "--body-file="+bodyFile)
	return strings.TrimSpace(string(b)), err
}

// Checks reads the draft, mergeable, head, and check state of pull request n.
func (g GH) Checks(ctx context.Context, n int) (Checks, error) {
	b, err := g.run(ctx, "pr", "view", strconv.Itoa(n), "--json=isDraft,mergeable,headRefOid,statusCheckRollup")
	if err != nil {
		return Checks{}, err
	}
	var raw struct {
		IsDraft           *bool   `json:"isDraft"`
		Mergeable         *string `json:"mergeable"`
		HeadRefOid        *string `json:"headRefOid"`
		StatusCheckRollup *[]struct {
			Typename   *string `json:"__typename"`
			Status     *string `json:"status"`
			Conclusion *string `json:"conclusion"`
			State      *string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Checks{}, invalidOutput("pr view", err)
	}
	if raw.IsDraft == nil || raw.Mergeable == nil || raw.HeadRefOid == nil || raw.StatusCheckRollup == nil {
		return Checks{}, invalidOutput("pr view", errors.New("a field is missing"))
	}
	c := Checks{IsDraft: *raw.IsDraft, Mergeable: *raw.Mergeable, HeadRefOid: *raw.HeadRefOid, Rollup: []Check{}}
	for _, r := range *raw.StatusCheckRollup {
		var check Check
		switch {
		case r.Typename == nil:
			return Checks{}, invalidOutput("pr view", errors.New("a check has no __typename"))
		case *r.Typename == "CheckRun" && r.Status != nil && r.Conclusion != nil:
			check = Check{Typename: *r.Typename, Status: *r.Status, Conclusion: *r.Conclusion}
		case *r.Typename == "StatusContext" && r.State != nil:
			check = Check{Typename: *r.Typename, State: *r.State}
		default:
			return Checks{}, invalidOutput("pr view", fmt.Errorf("a %s check is missing a field", *r.Typename))
		}
		c.Rollup = append(c.Rollup, check)
	}
	return c, nil
}

// Merge merges pull request n with a merge commit titled subject, only while
// its head is still head.
func (g GH) Merge(ctx context.Context, n int, head, subject string) error {
	_, err := g.run(ctx, "pr", "merge", strconv.Itoa(n), "--merge", "--match-head-commit="+head, "--subject="+subject)
	return err
}

// Merged reads the state, merge commit, and head of the pull request that
// selector names by number or URL. MergeCommit is "" while there is none.
func (g GH) Merged(ctx context.Context, selector string) (Merged, error) {
	b, err := g.run(ctx, "pr", "view", selector, "--json=state,mergeCommit,headRefOid")
	if err != nil {
		return Merged{}, err
	}
	var raw struct {
		State       *string `json:"state"`
		MergeCommit *struct {
			Oid *string `json:"oid"`
		} `json:"mergeCommit"`
		HeadRefOid *string `json:"headRefOid"`
	}
	if err := json.Unmarshal(b, &raw); err != nil || raw.State == nil || raw.HeadRefOid == nil {
		return Merged{}, invalidOutput("pr view", err)
	}
	m := Merged{State: *raw.State, HeadRefOid: *raw.HeadRefOid}
	if raw.MergeCommit != nil && raw.MergeCommit.Oid != nil {
		m.MergeCommit = *raw.MergeCommit.Oid
	}
	return m, nil
}

func invalidOutput(command string, err error) error {
	if err == nil {
		err = errors.New("unexpected shape")
	}
	return &cli.Error{Code: "land_gh_output_invalid", Message: fmt.Sprintf("gh %s returned unreadable JSON: %v", command, err)}
}
