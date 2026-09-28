// Package selector chooses live agents for commands that act on several:
// by explicit name, pane, or record id, or by a filter over live Herdr state
// and this repository's agent records.
// filter.go defines Filter and its validation; resolve.go lists and matches
// agents against a Filter; selection.go turns explicit targets or a filter
// into the targets a command addresses.
package selector
