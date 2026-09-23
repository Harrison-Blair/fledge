package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func paneRead() map[string]any {
	return map[string]any{"pane_id": "w1:p2", "workspace_id": "w1", "tab_id": "w1:t1", "source": "recent_unwrapped", "format": "text", "text": "hello\n", "revision": 7, "truncated": false}
}

func TestReadSendsParamsAndDecodes(t *testing.T) {
	for _, lines := range []*uint32{nil, ptr(uint32(0)), ptr(uint32(4294967295))} {
		want := map[string]any{"target": "w1:p2", "source": "recent_unwrapped", "format": "text"}
		if lines != nil {
			want["lines"] = *lines
		}
		c := Client{API: apiFunc(func(method string, params any) (any, error) {
			if method != "agent.read" || !reflect.DeepEqual(normalize(params), normalize(want)) {
				t.Fatalf("%s %#v", method, params)
			}
			return map[string]any{"type": "pane_read", "read": paneRead()}, nil
		})}
		r, err := c.Read(context.Background(), "w1:p2", "recent_unwrapped", lines)
		if err != nil || r.PaneID != "w1:p2" || r.Text != "hello\n" || *r.Revision != 7 || *r.Truncated {
			t.Fatalf("%v %+v", err, r)
		}
	}
}

func TestReadRejectsIncompleteResults(t *testing.T) {
	for _, field := range []string{"type", "pane_id", "workspace_id", "tab_id", "source", "format", "revision", "truncated"} {
		read := paneRead()
		result := map[string]any{"type": "pane_read", "read": read}
		switch field {
		case "type":
			result["type"] = "pane_info"
		case "source":
			read["source"] = "recent"
		case "format":
			read["format"] = "ansi"
		case "revision", "truncated":
			delete(read, field)
		default:
			read[field] = ""
		}
		c := Client{API: apiFunc(func(string, any) (any, error) { return result, nil })}
		_, err := c.Read(context.Background(), "w1:p2", "recent_unwrapped", nil)
		var remote *herdr.Error
		if !errors.As(err, &remote) || remote.Code != "protocol_error" {
			t.Fatalf("%s: %v", field, err)
		}
	}
}

// normalize compares params as the socket would encode them.
func normalize(v any) any {
	data, _ := json.Marshal(v)
	var out any
	json.Unmarshal(data, &out)
	return out
}
