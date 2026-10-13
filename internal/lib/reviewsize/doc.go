// Package reviewsize measures how large a change is for review and compares
// it with the review-unit limits of reference/plan/review-units.md. The
// report is advisory: it never refuses an over-limit change.
//
// A path with a testdata segment at any depth is a fixture: it adds no source
// lines, no guard-eligible file, and no code domain, as root testdata/ does
// not. Herdr raw captures under reference/herdr/raw/ are not guard-eligible.
//
//	classify.go  per-path source, test, guard, and domain classification
//	measure.go   git diff --numstat parsing and the summed Diff
//	report.go    limits, Compare, the JSON Report, and its human rendering
package reviewsize
