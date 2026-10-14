// Package landing holds the checks and Git and GitHub CLI access that task
// verify, create, import, and land share: commit and branch validation,
// delivery modes, the land lock, and strictly decoded gh output.
//
//	git.go   commit resolution, branch validation, refs, checkouts, and changes
//	mode.go  delivery mode validation and the configured default
//	lock.go  the repository's land lock
//	resolve.go  a task's branch, target, and base ref
//	patchid.go  patch ids and the carry-forward check of a rebased head
//	gh.go    gh pull request commands, the REST pull request read, the stack POST, the merge-async calls, and their decoded output
package landing
