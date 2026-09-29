package task

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

func session(kind, value string) *herdr.AgentSession {
	return &herdr.AgentSession{Kind: &kind, Value: &value}
}

func TestObserveStoresTheLiveSessionBestEffort(t *testing.T) {
	rec := identity.Record{ID: "0000aaaa"}
	stored := identity.Record{ID: "0000aaaa", NativeSession: &identity.NativeSessionRef{Value: "s1"}}
	now := time.Date(2026, 9, 24, 5, 0, 0, 0, time.UTC)
	var seen string
	ok := func(_ *state.Store, id string, s herdr.AgentSession, at time.Time) (identity.Record, bool, error) {
		seen = id + " " + *s.Value + " " + at.Format(time.RFC3339)
		return stored, true, nil
	}
	out := libagent.Outcome{}
	if got := Observe(nil, ok, rec, &herdr.AgentDetails{AgentSession: session("id", "s1")}, now, &out); !reflect.DeepEqual(got, stored) || seen != "0000aaaa s1 2026-09-24T05:00:00Z" ||
		!reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "updated", Kind: "native_session", ID: "0000aaaa"}}) {
		t.Fatalf("%+v %q %+v", got, seen, out.Effects)
	}
	failing := func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return identity.Record{}, false, errors.New("read-only")
	}
	out = libagent.Outcome{Status: "success"}
	if got := Observe(nil, failing, rec, &herdr.AgentDetails{AgentSession: session("id", "s1")}, now, &out); !reflect.DeepEqual(got, rec) || out.Error != nil ||
		!reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "warning", Kind: "native_session", ID: "0000aaaa"}}) {
		t.Fatalf("%+v %+v", got, out)
	}
	unchanged := func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return stored, false, nil
	}
	out = libagent.Outcome{}
	if got := Observe(nil, unchanged, rec, &herdr.AgentDetails{AgentSession: session("id", "s1")}, now, &out); !reflect.DeepEqual(got, stored) || len(out.Effects) != 0 {
		t.Fatalf("%+v %+v", got, out)
	}
	for _, live := range []*herdr.AgentDetails{nil, {}} {
		out = libagent.Outcome{}
		if got := Observe(nil, failing, rec, live, now, &out); !reflect.DeepEqual(got, rec) || len(out.Effects) != 0 {
			t.Fatalf("%+v %+v", got, out)
		}
	}
}

func TestRecordWithoutUsageUnmarshalsNil(t *testing.T) {
	var r Record
	if err := json.Unmarshal([]byte(`{"id":"0000aaaa","status":"completed"}`), &r); err != nil || r.Usage != nil {
		t.Fatalf("%+v %v", r.Usage, err)
	}
}
