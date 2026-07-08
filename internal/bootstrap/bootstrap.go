// Package bootstrap embeds the agent-neutral core skill and the per-harness
// adapter files that fledge init scaffolds into a target repository, so the
// orchestration layer ships with the binary.
//
// The core skill (core/skills/fledge-orchestrate, fledge-interrogate) is the
// single agent-neutral source of fledge's workflow. Each adapter
// (adapters/<harness>/manifest.yaml + files) is a thin, format-only mapping that
// tells one harness how to realize fledge's primitives. Adding or moving to a
// new harness is editing a manifest, not Go code.
package bootstrap

import "embed"

// FS holds the core skill tree and all adapter trees. The registry reads
// adapter manifests from it generically.
//
//go:embed core adapters
var FS embed.FS
