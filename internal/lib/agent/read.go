package agent

import (
	"context"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Read captures target's terminal text from the wire-spelled source. A nil
// lines requests Herdr's default extent. Reads do not mark output seen.
func (c Client) Read(ctx context.Context, target, source string, lines *uint32) (herdr.PaneRead, error) {
	params := map[string]any{"target": target, "source": source, "format": "text"}
	if lines != nil {
		params["lines"] = *lines
	}
	var r herdr.PaneReadResult
	err := c.Call(ctx, "agent.read", params, &r)
	if err == nil && (r.Type != "pane_read" || !ValidPane(herdr.Pane{PaneID: r.Read.PaneID, WorkspaceID: r.Read.WorkspaceID, TabID: r.Read.TabID}) ||
		r.Read.Source != source || r.Read.Format != "text" || r.Read.Revision == nil || r.Read.Truncated == nil) {
		err = Protocol("incomplete agent.read result")
	}
	if err != nil {
		return herdr.PaneRead{}, err
	}
	return r.Read, nil
}
