// Package profiles resolves named agent launch profiles: built-ins embedded in
// the binary, overlaid or extended by repository files in .fledge/profiles.
// profiles.go loads, resolves, and lists profiles; schema.go decodes and validates profile files;
// render.go renders a profile's Markdown brief; builtin/ holds the embedded built-in profiles
// and protocol.md, the shared Fledge protocol block.
package profiles
