// Package memory stores repository-scoped agent memories: one fact per
// Markdown file in the primary checkout's .fledge/memories, with a generated
// MEMORY.md index. memory.go defines the memory, its validation, file format,
// and index lines; store.go lists, reads, adds, and removes memory files under
// the state store lock and regenerates the index.
package memory
