// Package bootstrap prepares a freshly launched Herder session: it waits for
// the server, labels the root workspace and tab, reconciles all managed
// workspaces, and starts the chosen agent in the orchestrator pane.
//
// Files:
//   - bootstrap.go  server and reconciliation contracts, input, timing, and Run
package bootstrap
