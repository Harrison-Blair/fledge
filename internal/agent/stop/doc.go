// Package stop implements agent stop: tearing down live agents by closing
// their panes and ending their Fledge records.
// stop.go validates options, stops one target, and renders its result;
// multi.go resolves targets, stops several in order or plans a dry run, and renders one row per target.
package stop
