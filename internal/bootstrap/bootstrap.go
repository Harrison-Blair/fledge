// Package bootstrap embeds the .claude agents and skills that fledge init
// scaffolds into a target repository so its orchestration layer ships with
// the binary.
package bootstrap

import "embed"

//go:embed claude
var FS embed.FS
