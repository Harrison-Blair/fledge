package reviewsize

import (
	"path"
	"strings"
)

// class is what a changed path counts toward.
type class struct {
	source  bool   // its lines are hand-written source lines
	test    bool   // a test file: adds a code domain but no source lines
	guarded bool   // a guard-eligible file
	domain  string // its code domain, set for source and test files
	docArea string // its documentation area, set for other reference/<area>/ files
}

func classify(p string) class {
	dir, base := path.Split(p)
	dir = strings.TrimSuffix(dir, "/")
	if inTestdata(dir) {
		return class{}
	}
	c := class{guarded: !strings.HasPrefix(p, "reference/herdr/raw/")}
	switch {
	case strings.HasSuffix(base, "_test.go"):
		c.test = true
	case strings.HasSuffix(base, ".go"):
		c.source = true
	case dir == "internal/lib/profiles/builtin" && strings.HasSuffix(base, ".md"):
		c.source = true
	case dir == ".github/scripts" && strings.HasSuffix(base, "_test.py"):
		c.test = true
	case dir == ".github/scripts" && strings.HasSuffix(base, ".py"):
		c.source = true
	case dir == ".github/workflows" && strings.HasSuffix(base, ".yml"):
		c.source = true
	}
	if c.source || c.test {
		c.domain = domain(dir)
	} else if parts := strings.Split(dir, "/"); len(parts) >= 2 && parts[0] == "reference" {
		c.docArea = "reference/" + parts[1]
	}
	return c
}

// inTestdata reports whether dir has a testdata segment at any depth.
func inTestdata(dir string) bool {
	for _, s := range strings.Split(dir, "/") {
		if s == "testdata" {
			return true
		}
	}
	return false
}

// domain names the code domain of a source or test file in dir.
func domain(dir string) string {
	parts := strings.Split(dir, "/")
	switch {
	case dir == "":
		return "module root"
	case parts[0] == ".github":
		return ".github"
	case dir == "cmd":
		return "cmd root"
	case parts[0] == "internal" && len(parts) >= 3 && parts[1] == "lib":
		return "lib/" + parts[2]
	case (parts[0] == "cmd" || parts[0] == "internal") && len(parts) >= 2:
		return parts[1]
	}
	return parts[0]
}
