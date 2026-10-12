// Package landing holds the checks and Git and GitHub CLI access that task
// verify, create, import, and land share: commit and branch validation,
// delivery modes, the land lock, and strictly decoded gh output.
//
//	git.go   commit resolution, branch validation, refs, checkouts, and changes
//	mode.go  delivery mode validation and the configured default
//	lock.go  the repository's land lock
//	gh.go    gh pull request commands and their decoded output
package landing
