// Package catalog reports the model identifiers each supported agent harness
// accepts. Every lookup shells out to the harness CLI, so results reflect the
// models installed on this machine.
//
// Files:
//   - catalog.go   Harness identifiers and the Harnesses/Models entry points
//   - families.go  model family ranking used for ordering
//   - order.go     model identifier comparison and sorting
//   - parse.go     parsing of harness CLI output
package catalog
