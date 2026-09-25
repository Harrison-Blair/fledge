package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// labelAPI serves Label's requests for a pane alone in a tab of count panes
// labeled tabLabel, recording each method called.
func labelAPI(t *testing.T, count *int, tabLabel string, calls *[]string) apiFunc {
	p := herdr.Pane{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1"}
	tab := herdr.Tab{ID: "w1:t1", WorkspaceID: "w1", Label: tabLabel, PaneCount: count}
	return func(method string, params any) (any, error) {
		*calls = append(*calls, method)
		switch method {
		case "pane.rename":
			if !reflect.DeepEqual(params, map[string]any{"pane_id": "w1:p2", "label": "reviewer"}) {
				t.Fatalf("%#v", params)
			}
			return herdr.PaneResult{Type: "pane_info", Pane: p}, nil
		case "tab.get":
			return herdr.TabResult{Type: "tab_info", Tab: tab}, nil
		case "tab.rename":
			if !reflect.DeepEqual(params, map[string]any{"tab_id": "w1:t1", "label": "reviewer"}) {
				t.Fatalf("%#v", params)
			}
			tab.Label = "reviewer"
			return herdr.TabResult{Type: "tab_info", Tab: tab}, nil
		}
		t.Fatalf("unexpected %s", method)
		return nil, nil
	}
}

func TestLabelNamesPaneAndItsOwnTab(t *testing.T) {
	for label, tc := range map[string]struct {
		count    *int
		tabLabel string
		calls    []string
		effects  []Effect
	}{
		"sole pane":         {ptr(1), "1", []string{"pane.rename", "tab.get", "tab.rename"}, []Effect{{Action: "updated", Kind: "pane_label", ID: "w1:p2"}, {Action: "updated", Kind: "tab", ID: "w1:t1"}}},
		"shared tab":        {ptr(2), "1", []string{"pane.rename", "tab.get"}, []Effect{{Action: "updated", Kind: "pane_label", ID: "w1:p2"}}},
		"tab already named": {ptr(1), "reviewer", []string{"pane.rename", "tab.get"}, []Effect{{Action: "updated", Kind: "pane_label", ID: "w1:p2"}}},
	} {
		t.Run(label, func(t *testing.T) {
			var calls []string
			c := Client{API: labelAPI(t, tc.count, tc.tabLabel, &calls)}
			out := Outcome{Status: "success", Effects: []Effect{}}
			if err := c.Label(context.Background(), herdr.Pane{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1"}, "reviewer", &out); err != nil || out.Error != nil {
				t.Fatalf("%v %+v", err, out.Error)
			}
			if !reflect.DeepEqual(calls, tc.calls) || !reflect.DeepEqual(out.Effects, tc.effects) {
				t.Fatalf("%v %+v", calls, out.Effects)
			}
		})
	}
}

func TestLabelRejectsTabWithoutPaneCount(t *testing.T) {
	var calls []string
	c := Client{API: labelAPI(t, nil, "1", &calls)}
	out := Outcome{Status: "success", Effects: []Effect{}}
	err := c.Label(context.Background(), herdr.Pane{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1"}, "reviewer", &out)
	if err == nil || out.Error == nil || out.Error.Phase != "tab.get" || out.Error.Code != "protocol_error" || out.Status != "partial" {
		t.Fatalf("%v %+v %+v", err, out.Error, out)
	}
}
