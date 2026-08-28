// Package picker presents a filterable terminal selection list. Terminal
// detection is supplied by the CLI boundary so this package does not depend on
// a particular file-descriptor implementation.
//
// Files:
//   - picker.go   Option, ErrCancelled, and Select, the filterable
//     single-select Bubble Tea program
//   - chooser.go  AgentChooser, which walks harness and model selection to
//     produce a session.AgentChoice
package picker
