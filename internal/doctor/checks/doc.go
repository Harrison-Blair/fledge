// Package checks implements doctor's six read-only diagnoses.
// connectivity.go reports whether Herdr answered ping; compatibility.go compares
// the pong protocol with the pinned one; harness.go lists integration targets;
// models.go runs read-only model discovery; configuration.go defines Environment
// and inspects the Herdr environment variables and socket. branches.go reports
// whether the configured dev branch exists.
package checks
