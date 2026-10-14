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

// Pull is a pull request as the REST API reports it. MergeCommit is "" while
// merge_commit_sha is null. Stack is nil while the pull request is in no stack.
type Pull struct {
	Number                             int
	State                              string
	Merged, Draft                      bool
	MergeCommit                        string
	HeadSHA, HeadRef, BaseSHA, BaseRef string
	Stack                              *Stack
}

// Stack is the stack of a pull request: its number, its count of pull
// requests, and the position of the pull request, counted from 1 at the bottom.
type Stack struct{ Number, Size, Position int }

// run runs gh with args and returns its stdout. On an error it also returns
// a gh_failed error with its stderr.
func (g GH) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = g.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return b, &cli.Error{Code: "land_gh_failed", Message: fmt.Sprintf("gh %s failed: %v: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))}
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

// Pull reads pull request n through the REST API.
func (g GH) Pull(ctx context.Context, n int) (Pull, error) {
	b, err := g.run(ctx, "api", "--method=GET", "repos/{owner}/{repo}/pulls/"+strconv.Itoa(n))
	if err != nil {
		return Pull{}, apiError(err, b)
	}
	type ref struct {
		SHA *string `json:"sha"`
		Ref *string `json:"ref"`
	}
	var raw struct {
		Number         *int            `json:"number"`
		State          *string         `json:"state"`
		Merged         *bool           `json:"merged"`
		MergeCommitSHA json.RawMessage `json:"merge_commit_sha"`
		Head           *ref            `json:"head"`
		Base           *ref            `json:"base"`
		Draft          *bool           `json:"draft"`
		Stack          json.RawMessage `json:"stack"`
	}
	var merge *string
	var stack *struct {
		Number   *int `json:"number"`
		Size     *int `json:"size"`
		Position *int `json:"position"`
	}
	// A missing nullable field leaves its RawMessage nil, and null decodes to nil.
	if err := json.Unmarshal(b, &raw); err != nil {
		return Pull{}, invalidOutput("api pulls", err)
	}
	if raw.Number == nil || raw.State == nil || raw.Merged == nil || raw.MergeCommitSHA == nil || raw.Draft == nil || raw.Stack == nil ||
		raw.Head == nil || raw.Head.SHA == nil || raw.Head.Ref == nil || raw.Base == nil || raw.Base.SHA == nil || raw.Base.Ref == nil ||
		json.Unmarshal(raw.MergeCommitSHA, &merge) != nil || json.Unmarshal(raw.Stack, &stack) != nil ||
		stack != nil && (stack.Number == nil || stack.Size == nil || stack.Position == nil) {
		return Pull{}, invalidOutput("api pulls", errors.New("a field is missing"))
	}
	p := Pull{Number: *raw.Number, State: *raw.State, Merged: *raw.Merged, Draft: *raw.Draft,
		HeadSHA: *raw.Head.SHA, HeadRef: *raw.Head.Ref, BaseSHA: *raw.Base.SHA, BaseRef: *raw.Base.Ref}
	if merge != nil {
		p.MergeCommit = *merge
	}
	if stack != nil {
		p.Stack = &Stack{Number: *stack.Number, Size: *stack.Size, Position: *stack.Position}
	}
	return p, nil
}

// CreateStack forms a GitHub stack of the pull requests numbers, bottom
// first, and returns the stack number. It sends no base: GitHub takes the base
// of the bottom pull request. An HTTP 422 answer is land_stack_mismatch.
func (g GH) CreateStack(ctx context.Context, numbers []int) (int, error) {
	args := []string{"api", "--method=POST", "repos/{owner}/{repo}/stacks"}
	for _, n := range numbers {
		args = append(args, "--field=pull_requests[]="+strconv.Itoa(n))
	}
	b, err := g.run(ctx, args...)
	if err != nil {
		var answer struct{ Message, Status string }
		if json.Unmarshal(b, &answer) == nil && answer.Status == "422" {
			return 0, &cli.Error{Code: "land_stack_mismatch", Message: "GitHub refused the stack: " + answer.Message}
		}
		return 0, apiError(err, b)
	}
	var raw struct {
		Number *int `json:"number"`
	}
	if err := json.Unmarshal(b, &raw); err != nil || raw.Number == nil {
		return 0, invalidOutput("api stacks", err)
	}
	return *raw.Number, nil
}

// MergeAnswer is a merge-async answer. Status is pending, enqueued, merged,
// or failed. UUID names a pending request. SHA is the merge commit of a
// merged pull request. Message is the text of GitHub.
type MergeAnswer struct{ Status, UUID, SHA, Message string }

// ErrMergeRequestGone is an HTTP 404 answer to a poll. Fledge assumes that
// GitHub gives it for an expired uuid. The probes did not observe it.
var ErrMergeRequestGone = errors.New("GitHub has no merge request with this uuid")

// MergeAsync requests a merge commit of pull request n at head sha.
func (g GH) MergeAsync(ctx context.Context, n int, sha string) (MergeAnswer, error) {
	return g.mergeAnswer(ctx, true, "api", "--method=PUT", "repos/{owner}/{repo}/pulls/"+strconv.Itoa(n)+"/merge-async",
		"--raw-field=merge_method=merge", "--raw-field=sha="+sha)
}

// MergeStatus reads the status of merge request uuid of pull request n.
func (g GH) MergeStatus(ctx context.Context, n int, uuid string) (MergeAnswer, error) {
	return g.mergeAnswer(ctx, false, "api", "--method=GET", "repos/{owner}/{repo}/pulls/"+strconv.Itoa(n)+"/merge-async/"+uuid)
}

// mergeAnswer runs a merge-async call and decodes its answer, also from the
// body of HTTP 400. A pending answer to the PUT needs a uuid, a merged answer
// needs a full sha, and a failed answer needs a message.
func (g GH) mergeAnswer(ctx context.Context, put bool, args ...string) (MergeAnswer, error) {
	b, err := g.run(ctx, args...)
	var raw struct {
		Status  *string                              `json:"status"`
		Details *struct{ Message, UUID, SHA string } `json:"details"`
	}
	decoded := json.Unmarshal(b, &raw)
	if err != nil {
		switch {
		case !put && decoded == nil && raw.Status != nil && *raw.Status == "404":
			return MergeAnswer{}, ErrMergeRequestGone
		case decoded != nil || raw.Status == nil || *raw.Status != "failed":
			return MergeAnswer{}, apiError(err, b)
		}
	}
	if decoded != nil || raw.Status == nil {
		return MergeAnswer{}, invalidOutput("api merge-async", decoded)
	}
	a := MergeAnswer{Status: *raw.Status}
	if d := raw.Details; d != nil {
		a.Message, a.UUID, a.SHA = d.Message, d.UUID, d.SHA
	}
	switch {
	case a.Status == "pending" && put:
		if a.UUID != "" {
			return a, nil
		}
	case a.Status == "pending" || a.Status == "enqueued" && !put:
		return a, nil
	case a.Status == "merged":
		if FullHash(a.SHA) {
			return a, nil
		}
	case a.Status == "failed":
		if a.Message != "" {
			return a, nil
		}
	}
	return MergeAnswer{}, invalidOutput("api merge-async", fmt.Errorf("a %q answer is missing a field or has an unknown status", a.Status))
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

// apiError adds the message of the JSON body that gh api writes to stdout on
// an HTTP error to err.
func apiError(err error, body []byte) error {
	var raw struct {
		Message string `json:"message"`
	}
	var coded *cli.Error
	if json.Unmarshal(body, &raw) == nil && raw.Message != "" && errors.As(err, &coded) {
		coded.Message += ": GitHub answered: " + raw.Message
	}
	return err
}

func invalidOutput(command string, err error) error {
	if err == nil {
		err = errors.New("unexpected shape")
	}
	return &cli.Error{Code: "land_gh_output_invalid", Message: fmt.Sprintf("gh %s returned unreadable JSON: %v", command, err)}
}
