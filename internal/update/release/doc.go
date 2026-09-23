// Package release finds Fledge's latest stable release and downloads its
// checksum-verified archive.
// source.go performs bounded GitHub requests (LatestTag, Artifact); checksum.go
// names release assets and verifies them against checksums.txt; semver.go
// parses and compares stable release tags.
package release
