// Package picker resolves agent launch choices and presents filterable terminal
// selection lists. Terminal detection is supplied by the CLI boundary so this
// package does not depend on a particular file-descriptor implementation.
//
// Files:
//   - picker.go   Option, ErrCancelled, and Select, the filterable
//     single-select Bubble Tea program
//   - chooser.go  Resolver launch precedence, profile/harness/model prompts,
//     and session lifecycle adapters
package picker
