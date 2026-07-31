package fledge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

type SessionPruneFailure struct {
	Session string `json:"session"`
	Error   string `json:"error"`
}

type SessionPruneResult struct {
	Candidates []string              `json:"candidates"`
	Deleted    []string              `json:"deleted"`
	Failures   []SessionPruneFailure `json:"failures,omitempty"`
	DryRun     bool                  `json:"dry_run"`
	Cancelled  bool                  `json:"cancelled"`
}

// PruneCandidates returns sorted stopped, non-default named sessions eligible
// for pruning. Unless all is true, only the reserved fledge- namespace is
// selected.
func PruneCandidates(ctx context.Context, binary herdr.Binary, all bool) ([]string, error) {
	sessions, err := binary.Sessions(ctx)
	if err != nil {
		return nil, Wrap("herdr_discovery_failed", err.Error(), err)
	}
	candidates := make([]string, 0)
	for _, session := range sessions {
		if session.Name == "" || session.Running || session.Default {
			continue
		}
		if !all && !strings.HasPrefix(session.Name, "fledge-") {
			continue
		}
		candidates = append(candidates, session.Name)
	}
	sort.Strings(candidates)
	return candidates, nil
}

// PruneSessions deletes every candidate, continuing after individual errors.
func PruneSessions(ctx context.Context, binary herdr.Binary, candidates []string) (SessionPruneResult, error) {
	result := SessionPruneResult{
		Candidates: append([]string(nil), candidates...),
		Deleted:    []string{},
	}
	for _, session := range candidates {
		if err := binary.DeleteSession(ctx, session); err != nil {
			result.Failures = append(result.Failures, SessionPruneFailure{Session: session, Error: err.Error()})
			continue
		}
		result.Deleted = append(result.Deleted, session)
	}
	if len(result.Failures) > 0 {
		messages := make([]string, 0, len(result.Failures))
		for _, failure := range result.Failures {
			messages = append(messages, failure.Session+": "+failure.Error)
		}
		return result, &Error{
			Code: "session_prune_failed",
			Message: fmt.Sprintf("failed to delete %d of %d Herdr sessions: %s",
				len(result.Failures), len(candidates), strings.Join(messages, "; ")),
			Details: result,
		}
	}
	return result, nil
}
