package agent

import (
	"fmt"
	"slices"
	"strings"
)

// TargetFailure is one failed target of a fan-out and its failure code.
type TargetFailure struct{ Target, Code string }

// FanOutFailure summarizes the failed targets of total, or returns nil when
// none failed. It names each failed target and its code; one shared code is
// kept, several become operation_failed. Phrase says how the targets failed.
func FanOutFailure(failed []TargetFailure, total int, phrase, phase string) *Failure {
	if len(failed) == 0 {
		return nil
	}
	var names, codes []string
	for _, f := range failed {
		names = append(names, fmt.Sprintf("%s (%s)", f.Target, f.Code))
		if !slices.Contains(codes, f.Code) {
			codes = append(codes, f.Code)
		}
	}
	code := "operation_failed"
	if len(codes) == 1 {
		code = codes[0]
	}
	return &Failure{Code: code, Message: fmt.Sprintf("%d of %d targets %s: %s", len(failed), total, phrase, strings.Join(names, ", ")), Phase: phase}
}
