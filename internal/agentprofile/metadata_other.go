//go:build !unix

package agentprofile

import "io/fs"

func requireCurrentOwner(fs.FileInfo) error { return nil }

func requireSingleLink(fs.FileInfo) error { return nil }
