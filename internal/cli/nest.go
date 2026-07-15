package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/nest"
	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("nest", runNest,
		"fledge nest new <doc> | scaffold | scout --module <m> | stamp <file> | status [flags]")
}

func runNest(args []string) int {
	if len(args) == 0 {
		return usageErr("usage: fledge nest new|scaffold|scout|stamp ...")
	}
	verb := args[0]
	switch verb {
	case "new":
		return runNestNew(args[1:])
	case "scaffold":
		return runNestScaffold(args[1:])
	case "scout":
		return runNestScout(args[1:])
	case "stamp":
		return runNestStamp(args[1:])
	case "status":
		return runNestStatus(args[1:])
	default:
		return usageErr("fledge nest: unknown verb %q (available: new, scaffold, scout, stamp, status)", verb)
	}
}

func runNestScaffold(args []string) int {
	fs := flag.NewFlagSet("nest scaffold", flag.ContinueOnError)
	agent := fs.String("agent", "fledge-forager", "agent name recorded in frontmatter")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if _, err := parseMixed(fs, args); err != nil {
		return ExitUsage
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	dir := r.ContextDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("%v", err)
	}
	if err := nest.ClearNest(dir); err != nil {
		return fail("%v", err)
	}

	generated := time.Now().UTC().Format(time.RFC3339)
	commit := r.Head()
	version := r.Version(binaryVersion)

	var created []string
	for _, docName := range nest.ConcernDocs {
		var body []byte
		if docName == "index" {
			body = nest.IndexBody()
		} else {
			body = nest.ConcernBody(nest.Title(docName))
		}
		d := &nest.Doc{
			Kind:          nest.Concern,
			Generated:     generated,
			Commit:        commit,
			Agent:         *agent,
			FledgeVersion: version,
			Body:          body,
		}
		path := filepath.Join(dir, docName+".md")
		if err := spec.WriteFileAtomic(path, d.Render()); err != nil {
			return fail("%v", err)
		}
		created = append(created, relPath(r.Root, path))
	}

	if *jsonOut {
		return emitJSON(map[string][]string{"created": created})
	}
	for _, c := range created {
		fmt.Printf("created %s\n", c)
	}
	return ExitOK
}

func runNestScout(args []string) int {
	fs := flag.NewFlagSet("nest scout", flag.ContinueOnError)
	module := fs.String("module", "", "module name (required)")
	agent := fs.String("agent", "fledge-context-scout", "agent name recorded in frontmatter")
	force := fs.Bool("force", false, "overwrite existing file")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if _, err := parseMixed(fs, args); err != nil {
		return ExitUsage
	}
	if *module == "" {
		return usageErr("fledge nest scout: --module is required")
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	authored := time.Now().UTC().Format(time.RFC3339)
	version := r.Version(binaryVersion)

	d := &nest.Doc{
		Kind:          nest.Scout,
		Module:        *module,
		Authored:      authored,
		Agent:         *agent,
		FledgeVersion: version,
		Body:          nest.ScoutBody(*module),
	}

	path := filepath.Join(r.ContextDir(), "raw", *module+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail("%v", err)
	}

	if *force {
		if err := spec.WriteFileAtomic(path, d.Render()); err != nil {
			return fail("%v", err)
		}
	} else {
		f, ferr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if ferr != nil {
			return fail("%s already exists; use --force to overwrite", *module)
		}
		if _, ferr = f.Write(d.Render()); ferr != nil {
			f.Close()
			return fail("%v", ferr)
		}
		if ferr = f.Close(); ferr != nil {
			return fail("%v", ferr)
		}
	}

	rel := relPath(r.Root, path)
	if *jsonOut {
		return emitJSON(map[string]string{"path": rel})
	}
	fmt.Printf("created %s\n", rel)
	return ExitOK
}

func runNestStamp(args []string) int {
	fs := flag.NewFlagSet("nest stamp", flag.ContinueOnError)
	kindFlag := fs.String("kind", "", "concern|scout (default: detect by path)")
	agent := fs.String("agent", "", "override agent field")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) == 0 {
		return usageErr("usage: fledge nest stamp <file>")
	}

	r, rErr := repo.Find()
	if rErr != nil {
		return envErr("%v", rErr)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	absPath, err := filepath.Abs(positional[0])
	if err != nil {
		return fail("%v", err)
	}

	nestDir := r.ContextDir()
	rel, relErr := filepath.Rel(nestDir, absPath)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return usageErr("fledge nest stamp: %s is outside .fledge/nest/", positional[0])
	}

	var docKind nest.Kind
	switch *kindFlag {
	case "concern":
		docKind = nest.Concern
	case "scout":
		docKind = nest.Scout
	case "":
		if strings.HasPrefix(rel, "raw"+string(filepath.Separator)) || rel == "raw" {
			docKind = nest.Scout
		} else {
			docKind = nest.Concern
		}
	default:
		return usageErr("fledge nest stamp: --kind must be concern or scout, got %q", *kindFlag)
	}

	b, err := os.ReadFile(absPath)
	if err != nil {
		return fail("%v", err)
	}

	generated := time.Now().UTC().Format(time.RFC3339)
	commit := r.Head()
	version := r.Version(binaryVersion)

	refreshed, err := nest.RefreshDoc(b, docKind, generated, commit, *agent, version)
	if err != nil {
		return fail("%v", err)
	}

	if err := spec.WriteFileAtomic(absPath, refreshed); err != nil {
		return fail("%v", err)
	}

	relStr := relPath(r.Root, absPath)
	if *jsonOut {
		return emitJSON(map[string]string{"path": relStr})
	}
	fmt.Printf("stamped %s\n", relStr)
	return ExitOK
}

// runNestStatus reports whether .fledge/nest/ holds a completed synthesis. It is
// the deterministic done-check the forager gates its final message on and the
// commissioner uses to distinguish a stall from a finished-but-unannounced
// forager. Exit code: ExitOK when complete, ExitFail when incomplete.
func runNestStatus(args []string) int {
	fs := flag.NewFlagSet("nest status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if _, err := parseMixed(fs, args); err != nil {
		return ExitUsage
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	res := nest.Status(r.ContextDir(), r.Head())

	if *jsonOut {
		if code := emitJSON(res); code != ExitOK {
			return code
		}
	} else if res.Complete {
		fmt.Println("nest complete: all concern docs synthesized, index stamped to HEAD")
	} else {
		fmt.Println("nest incomplete:")
		if len(res.MissingDocs) > 0 {
			fmt.Printf("  missing: %s\n", strings.Join(res.MissingDocs, ", "))
		}
		if len(res.StubDocs) > 0 {
			fmt.Printf("  not synthesized (still template stubs): %s\n", strings.Join(res.StubDocs, ", "))
		}
		if !res.IndexCommitMatches {
			fmt.Printf("  index stale: index commit %q != HEAD %q\n", res.IndexCommit, res.Head)
		}
	}

	if res.Complete {
		return ExitOK
	}
	return ExitFail
}

func runNestNew(args []string) int {
	fs := flag.NewFlagSet("nest new", flag.ContinueOnError)
	agent := fs.String("agent", "fledge-forager", "agent name recorded in frontmatter")
	force := fs.Bool("force", false, "overwrite existing file")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) == 0 {
		return usageErr("usage: fledge nest new <doc>")
	}
	docName := positional[0]
	if !nest.IsKnownDoc(docName) {
		return usageErr("fledge nest new: unknown doc %q (known: %s)",
			docName, strings.Join(nest.ConcernDocs, ", "))
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	generated := time.Now().UTC().Format(time.RFC3339)
	commit := r.Head()
	version := r.Version(binaryVersion)

	var body []byte
	if docName == "index" {
		body = nest.IndexBody()
	} else {
		body = nest.ConcernBody(nest.Title(docName))
	}

	d := &nest.Doc{
		Kind:          nest.Concern,
		Generated:     generated,
		Commit:        commit,
		Agent:         *agent,
		FledgeVersion: version,
		Body:          body,
	}
	content := d.Render()

	dir := r.ContextDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("%v", err)
	}
	path := filepath.Join(dir, docName+".md")

	if *force {
		if err := spec.WriteFileAtomic(path, content); err != nil {
			return fail("%v", err)
		}
	} else {
		f, ferr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if ferr != nil {
			return fail("%s already exists; use --force to overwrite", docName)
		}
		if _, ferr = f.Write(content); ferr != nil {
			f.Close()
			return fail("%v", ferr)
		}
		if ferr = f.Close(); ferr != nil {
			return fail("%v", ferr)
		}
	}

	rel := relPath(r.Root, path)
	if *jsonOut {
		return emitJSON(map[string]string{"path": rel})
	}
	fmt.Printf("created %s\n", rel)
	return ExitOK
}
