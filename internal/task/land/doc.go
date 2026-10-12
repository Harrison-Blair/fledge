// Package land implements task land: delivering a verified task's branch to
// its target branch, by a local merge or through a pull request, under the
// repository's land lock, and moving the task to landed.
//
//	land.go   options, result, task and guard checks, locked record writes, rendering
//	local.go  local mode: the merge in the primary checkout and its verified undo
//	pr.go     pr mode: push, pull request, checks, merge, and confirmation through gh
package land
