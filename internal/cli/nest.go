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
		"fledge nest new <doc> [--agent <s>] [--force] [--json]  (docs: "+strings.Join(nest.ConcernDocs, ", ")+")")
}

func runNest(args []string) int {
	if len(args) == 0 {
		return usageErr("usage: fledge nest new <doc>")
	}
	verb := args[0]
	switch verb {
	case "new":
		return runNestNew(args[1:])
	default:
		return usageErr("fledge nest: unknown verb %q (available: new)", verb)
	}
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
