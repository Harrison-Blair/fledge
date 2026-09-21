// Package agent implements Fledge agent operations and their presentation.
// options.go validates flags and native arguments; service.go orchestrates operations, including agent stop;
// get.go inspects live agents without side effects; pause.go interrupts foreground turns;
// placement.go resolves layout; worktree.go manages checkout preparation;
// models.go discovers local harness models; output.go renders stable outcomes.
package agent
