// Package record persists the on-disk session records of a Fledge project
// checkout and generates the Herder session names stored in them.
//
// Files:
//   - registry.go     Record and its Load/Create/Claim/ClearPending/Unclaim operations
//   - profile.go      immutable managed-profile snapshot and instruction artifact
//   - stop_intent.go  stop-intent sidecar written before an explicit stop
//   - naming.go       Slug and GenerateName plus Herder session-name validation
package record
