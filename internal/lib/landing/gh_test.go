package landing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

// fakeAPI puts a gh on PATH that logs its arguments to dir/log, writes
// dir/body to stdout, and exits with the status in dir/status.
func fakeAPI(t *testing.T, body string, status int) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + dir + "/log'\ncat '" + dir + "/body'\nexit " + string(rune('0'+status)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "body"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

const pullJSON = `{"number":11,"state":"open","merged":false,"merge_commit_sha":null,"draft":false,` +
	`"head":{"sha":"aaaa","ref":"l2"},"base":{"sha":"bbbb","ref":"l1"},` +
	`"stack":{"id":2120687,"number":5,"base":{"ref":"dev","sha":"cccc"},"size":3,"position":2}}`

func TestPullDecodesTheRESTPullRequest(t *testing.T) {
	dir := fakeAPI(t, pullJSON, 0)
	p, err := GH{Dir: dir}.Pull(context.Background(), 11)
	want := Pull{Number: 11, State: "open", HeadSHA: "aaaa", HeadRef: "l2", BaseSHA: "bbbb", BaseRef: "l1", Stack: &Stack{Number: 5, Size: 3, Position: 2}}
	if err != nil || p.Stack == nil || *p.Stack != *want.Stack {
		t.Fatalf("%v %+v", err, p)
	}
	p.Stack, want.Stack = nil, nil
	if p != want {
		t.Fatalf("%+v", p)
	}
	if log, _ := os.ReadFile(filepath.Join(dir, "log")); string(log) != "api --method=GET repos/{owner}/{repo}/pulls/11\n" {
		t.Fatalf("%q", log)
	}
	merged := strings.NewReplacer(`"merged":false`, `"merged":true`, `"merge_commit_sha":null`, `"merge_commit_sha":"dddd"`, `"draft":false`, `"draft":true`).Replace(pullJSON)
	merged = merged[:strings.Index(merged, `"stack"`)] + `"stack":null}`
	dir = fakeAPI(t, merged, 0)
	if p, err := (GH{Dir: dir}).Pull(context.Background(), 11); err != nil || !p.Merged || p.MergeCommit != "dddd" || !p.Draft || p.Stack != nil {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestPullRefusesAMissingField(t *testing.T) {
	for _, field := range []string{`"number":11,`, `"state":"open",`, `"merged":false,`, `"merge_commit_sha":null,`, `"draft":false,`,
		`"sha":"aaaa",`, `"ref":"l2"`, `"head":{"sha":"aaaa","ref":"l2"},`, `"base":{"sha":"bbbb","ref":"l1"},`, `"sha":"bbbb",`, `"ref":"l1"`,
		`"number":5,`, `"size":3,`, `,"position":2`} {
		body := strings.Replace(pullJSON, field, "", 1)
		if field == `"ref":"l2"` || field == `"ref":"l1"` {
			body = strings.Replace(strings.Replace(pullJSON, `,`+field, "", 1), field, "", 1)
		}
		if body == pullJSON {
			t.Fatalf("%s: not removed", field)
		}
		dir := fakeAPI(t, body, 0)
		var coded *cli.Error
		if _, err := (GH{Dir: dir}).Pull(context.Background(), 11); !errors.As(err, &coded) || coded.Code != "land_gh_output_invalid" {
			t.Fatalf("%s: %v", field, err)
		}
	}
	dir := fakeAPI(t, pullJSON[:strings.Index(pullJSON, `,"stack"`)]+"}", 0)
	var coded *cli.Error
	if _, err := (GH{Dir: dir}).Pull(context.Background(), 11); !errors.As(err, &coded) || coded.Code != "land_gh_output_invalid" {
		t.Fatalf("no stack: %v", err)
	}
}

// TestPullReadsTheBodyOfAnHTTPError: gh api exits nonzero on an HTTP error
// and writes the JSON body to stdout. The error has the GitHub message.
func TestPullReadsTheBodyOfAnHTTPError(t *testing.T) {
	dir := fakeAPI(t, `{"message":"Not Found","status":"404"}`, 1)
	var coded *cli.Error
	if _, err := (GH{Dir: dir}).Pull(context.Background(), 11); !errors.As(err, &coded) || coded.Code != "land_gh_failed" || !strings.Contains(coded.Message, "Not Found") {
		t.Fatalf("%v", err)
	}
}

// TestCreateStackPostsThePullRequestsBottomFirst: the POST body has the pull
// request numbers in order and no base. The answer gives the stack number.
func TestCreateStackPostsThePullRequestsBottomFirst(t *testing.T) {
	dir := fakeAPI(t, `{"id":2120746,"number":17,"base":{"ref":"dev"},"open":true,"pull_requests":[]}`, 0)
	n, err := GH{Dir: dir}.CreateStack(context.Background(), []int{3, 1, 2})
	if err != nil || n != 17 {
		t.Fatalf("%v %d", err, n)
	}
	want := "api --method=POST repos/{owner}/{repo}/stacks --field=pull_requests[]=3 --field=pull_requests[]=1 --field=pull_requests[]=2\n"
	if log, _ := os.ReadFile(filepath.Join(dir, "log")); string(log) != want {
		t.Fatalf("%q", log)
	}
}

func TestCreateStackRefusesAnUnknownShape(t *testing.T) {
	for _, body := range []string{`{"id":2120746,"open":true}`, `{"number":"17"}`, `[]`, `not json`} {
		dir := fakeAPI(t, body, 0)
		var coded *cli.Error
		if _, err := (GH{Dir: dir}).CreateStack(context.Background(), []int{1, 2}); !errors.As(err, &coded) || coded.Code != "land_gh_output_invalid" {
			t.Fatalf("%s: %v", body, err)
		}
	}
}

// TestCreateStackReadsTheBodyOfAnHTTPError: an HTTP 422 answer is
// land_stack_mismatch with the GitHub message. Another HTTP error is
// land_gh_failed with the GitHub message.
func TestCreateStackReadsTheBodyOfAnHTTPError(t *testing.T) {
	dir := fakeAPI(t, `{"message":"Pull requests #1, #2 are already part of a stack","documentation_url":"https://docs.github.com/rest/pulls/stacks#create-a-pull-request-stack","status":"422"}`, 1)
	var coded *cli.Error
	if _, err := (GH{Dir: dir}).CreateStack(context.Background(), []int{1, 2}); !errors.As(err, &coded) || coded.Code != "land_stack_mismatch" ||
		!strings.Contains(coded.Message, "Pull requests #1, #2 are already part of a stack") {
		t.Fatalf("%v", err)
	}
	dir = fakeAPI(t, `{"message":"Not Found","status":"404"}`, 1)
	if _, err := (GH{Dir: dir}).CreateStack(context.Background(), []int{1, 2}); !errors.As(err, &coded) || coded.Code != "land_gh_failed" || !strings.Contains(coded.Message, "Not Found") {
		t.Fatalf("%v", err)
	}
}

const (
	mergeUUID    = "d3a8ff89-475a-464b-9c87-02dc01da37ec"
	mergeSHA     = "7a4be7028342660f46d0c0869c8c74c436df9cb1"
	putPending   = `{"status":"pending","details":{"message":"Merge request enqueued.","uuid":"` + mergeUUID + `","merge_method":"merge","merge_action":"default","expected_head_sha":"e0954a8e013aae0ce9290709bc0e00735f077617","bypass_rules":false}}`
	getPending   = `{"status":"pending","details":{"message":"Merge request is in progress."}}`
	getMerged    = `{"status":"merged","details":{"message":"Pull request was merged.","sha":"` + mergeSHA + `"}}`
	putFailed    = `{"status":"failed","details":{"message":"Pull request head branch was modified."}}`
	putAlready   = `{"status":"merged","details":{"message":"Pull request is already merged.","sha":"` + mergeSHA + `"}}`
	mergeHeadSHA = "e0954a8e013aae0ce9290709bc0e00735f077617"
)

// TestMergeAsyncDecodesEachAnswer: the PUT sends the merge method and the
// head. Each answer gives the fields of its status. The failed answer comes
// with HTTP 400, so gh exits 1.
func TestMergeAsyncDecodesEachAnswer(t *testing.T) {
	for _, c := range []struct {
		body   string
		status int
		want   MergeAnswer
	}{
		{putPending, 0, MergeAnswer{Status: "pending", UUID: mergeUUID, Message: "Merge request enqueued."}},
		{putAlready, 0, MergeAnswer{Status: "merged", SHA: mergeSHA, Message: "Pull request is already merged."}},
		{putFailed, 1, MergeAnswer{Status: "failed", Message: "Pull request head branch was modified."}},
	} {
		dir := fakeAPI(t, c.body, c.status)
		got, err := GH{Dir: dir}.MergeAsync(context.Background(), 2, mergeHeadSHA)
		if err != nil || got != c.want {
			t.Fatalf("%s: %v %+v", c.body, err, got)
		}
		want := "api --method=PUT repos/{owner}/{repo}/pulls/2/merge-async --raw-field=merge_method=merge --raw-field=sha=" + mergeHeadSHA + "\n"
		if log, _ := os.ReadFile(filepath.Join(dir, "log")); string(log) != want {
			t.Fatalf("%q", log)
		}
	}
}

// TestMergeStatusDecodesEachAnswer: the GET names the uuid. A pending or
// enqueued answer needs only the status.
func TestMergeStatusDecodesEachAnswer(t *testing.T) {
	for _, c := range []struct {
		body   string
		status int
		want   MergeAnswer
	}{
		{getPending, 0, MergeAnswer{Status: "pending", Message: "Merge request is in progress."}},
		{`{"status":"enqueued"}`, 0, MergeAnswer{Status: "enqueued"}},
		{getMerged, 0, MergeAnswer{Status: "merged", SHA: mergeSHA, Message: "Pull request was merged."}},
		{putFailed, 1, MergeAnswer{Status: "failed", Message: "Pull request head branch was modified."}},
	} {
		dir := fakeAPI(t, c.body, c.status)
		got, err := GH{Dir: dir}.MergeStatus(context.Background(), 2, mergeUUID)
		if err != nil || got != c.want {
			t.Fatalf("%s: %v %+v", c.body, err, got)
		}
		if log, _ := os.ReadFile(filepath.Join(dir, "log")); string(log) != "api --method=GET repos/{owner}/{repo}/pulls/2/merge-async/"+mergeUUID+"\n" {
			t.Fatalf("%q", log)
		}
	}
}

// TestMergeAnswerRefusesAnUnknownShape: an unknown status or a missing
// required field is land_gh_output_invalid. A PUT pending answer needs a
// uuid, and a merged answer needs a 40-hex sha.
func TestMergeAnswerRefusesAnUnknownShape(t *testing.T) {
	for _, c := range []struct {
		body string
		put  bool
	}{
		{`{"status":"pending","details":{"message":"Merge request enqueued."}}`, true},
		{`{"status":"pending"}`, true},
		{`{"status":"merged","details":{"message":"Pull request was merged."}}`, false},
		{`{"status":"merged","details":{"sha":"7a4be70"}}`, false},
		{`{"status":"merged","details":{"sha":"` + strings.ToUpper(mergeSHA) + `"}}`, true},
		{`{"status":"failed","details":{}}`, false},
		{`{"status":"failed"}`, true},
		{`{"status":"done","details":{"sha":"` + mergeSHA + `"}}`, false},
		{`{"details":{"uuid":"` + mergeUUID + `"}}`, true},
		{`not json`, false},
	} {
		dir := fakeAPI(t, c.body, 0)
		g := GH{Dir: dir}
		var err error
		if c.put {
			_, err = g.MergeAsync(context.Background(), 2, mergeHeadSHA)
		} else {
			_, err = g.MergeStatus(context.Background(), 2, mergeUUID)
		}
		var coded *cli.Error
		if !errors.As(err, &coded) || coded.Code != "land_gh_output_invalid" {
			t.Fatalf("%s: %v", c.body, err)
		}
	}
}

// TestMergeAsyncReadsTheBodyOfAnHTTPError: an HTTP error that is not a
// failed answer is land_gh_failed with the GitHub message.
func TestMergeAsyncReadsTheBodyOfAnHTTPError(t *testing.T) {
	dir := fakeAPI(t, `{"message":"Server Error","status":"500"}`, 1)
	var coded *cli.Error
	if _, err := (GH{Dir: dir}).MergeAsync(context.Background(), 2, mergeHeadSHA); !errors.As(err, &coded) || coded.Code != "land_gh_failed" || !strings.Contains(coded.Message, "Server Error") {
		t.Fatalf("%v", err)
	}
	if _, err := (GH{Dir: dir}).MergeStatus(context.Background(), 2, mergeUUID); !errors.As(err, &coded) || coded.Code != "land_gh_failed" || !strings.Contains(coded.Message, "Server Error") {
		t.Fatalf("%v", err)
	}
}
