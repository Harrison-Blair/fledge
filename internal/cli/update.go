package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// updateAPIBaseURL is the GitHub API base URL queried for the latest
// release. Overridable in tests.
var updateAPIBaseURL = "https://api.github.com"

// updateExecutablePath resolves the path of the running binary. Exposed as a
// seam for FTHR-027, which builds the real download/verify/swap on top of it;
// unused beyond that in this feather.
var updateExecutablePath = os.Executable

func init() { register("update", runUpdate, "fledge update [--yes] [--json]") }

// githubRelease is the subset of the GitHub releases/latest response fledge
// needs.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// updateJSON is the --json dry-run payload.
type updateJSON struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	UpToDate bool   `json:"upToDate"`
	Notes    string `json:"notes"`
}

func runUpdate(args []string) int {
	return runUpdateWith(args, os.Stdin, os.Stdout)
}

func runUpdateWith(args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	jsonOut := fs.Bool("json", false, "machine-readable dry-run output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	rel, err := fetchLatestRelease(updateAPIBaseURL)
	if err != nil {
		return fail("checking for update: %v", err)
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	current := binaryVersion
	upToDate := latest == current

	if *jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(updateJSON{
			Current:  current,
			Latest:   latest,
			UpToDate: upToDate,
			Notes:    rel.Body,
		}); err != nil {
			return fail("encoding update status: %v", err)
		}
		return ExitOK
	}

	if upToDate {
		fmt.Fprintf(out, "fledge is already up to date (v%s)\n", current)
		return ExitOK
	}

	fmt.Fprintf(out, "current version: v%s\n", current)
	fmt.Fprintf(out, "latest version:  v%s\n", latest)
	if rel.Body != "" {
		fmt.Fprintf(out, "\nrelease notes:\n%s\n", rel.Body)
	}

	confirmed := *yes
	if !confirmed {
		confirmed = promptYesNo(in, out, fmt.Sprintf("Update to v%s? [y/N]: ", latest))
	}
	if !confirmed {
		return ExitOK
	}

	fmt.Fprintln(out, "(update mechanics not yet implemented)")
	return ExitOK
}

// fetchLatestRelease fetches and decodes the latest GitHub release from
// {baseURL}/repos/Harrison-Blair/fledge/releases/latest.
func fetchLatestRelease(baseURL string) (*githubRelease, error) {
	url := fmt.Sprintf("%s/repos/Harrison-Blair/fledge/releases/latest", baseURL)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s fetching latest release", resp.Status)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &rel, nil
}
