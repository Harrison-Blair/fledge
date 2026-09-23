// Package update finds and installs verified Linux release binaries.
//
// run.go validates the platform and sequences the child packages: release finds
// and downloads the verified archive, confirm asks for approval, archive
// extracts the executable, and install replaces the running binary.
package update
