// Package land implements task land: delivering a verified task's branch to
// its target branch, by a local merge or through a pull request, under the
// repository's land lock, and moving the task to landed.
//
//	land.go   options, result, task and guard checks, locked record writes, rendering
//	local.go  local mode: the merge in the target's checkout and its verified undo
//	pr.go     pr mode: push, pull request, checks, merge, and confirmation through gh
//	stack.go  stack parents: the parent guards, the layer checks and pushes, the layer pull requests, and their GitHub stack
package land
