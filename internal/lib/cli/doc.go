// Package cli holds process-output contracts shared by command implementations.
// cli.go marks rendered errors and output failures for the root command;
// outcome.go defines the result envelope, failure classification, and generic rendering;
// phase.go locates a failure at an operation phase;
// input.go reads inline, file, or stdin text;
// value.go converts optional values for JSON and human output.
package cli
