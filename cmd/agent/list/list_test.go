package list

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"fledge/internal/herdr"
)

func fixedAgents() []herdr.Agent {
	return []herdr.Agent{
		{Name: "reviewer", Kind: "claude", Status: "idle", WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"},
		{Name: "", Kind: "codex", Status: "busy", WorkspaceID: "ws1", TabID: "ws1:tab4", PaneID: "ws1:tab4:pane1"},
	}
}

func TestListPrintsAlignedTable(t *testing.T) {
	command := newCommand(func(context.Context) ([]herdr.Agent, error) {
		return fixedAgents(), nil
	}, func(context.Context) (json.RawMessage, error) {
		t.Fatal("raw list operation called without --json")
		return nil, nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "NAME      HARNESS  STATUS  WORKSPACE  TAB       PANE\n" +
		"reviewer  claude   idle    ws1        ws1:tab2  ws1:tab2:pane3\n" +
		"-         codex    busy    ws1        ws1:tab4  ws1:tab4:pane1\n"
	if output.String() != want {
		t.Fatalf("output =\n%q\nwant\n%q", output.String(), want)
	}
}

func TestListPrintsHeaderWhenNoAgentsExist(t *testing.T) {
	command := newCommand(func(context.Context) ([]herdr.Agent, error) {
		return nil, nil
	}, nil)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.String() != "NAME  HARNESS  STATUS  WORKSPACE  TAB  PANE\n" {
		t.Fatalf("output = %q, want the header alone", output.String())
	}
}

func TestListJSONPrintsRawHerderResult(t *testing.T) {
	command := newCommand(func(context.Context) ([]herdr.Agent, error) {
		t.Fatal("table list operation called with --json")
		return nil, nil
	}, func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"type":"agent_list","agents":[]}`), nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--json"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := `{"type":"agent_list","agents":[]}` + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestListRejectsArgumentsAndPropagatesErrors(t *testing.T) {
	t.Run("arguments", func(t *testing.T) {
		command := newCommand(func(context.Context) ([]herdr.Agent, error) {
			t.Fatal("list operation called")
			return nil, nil
		}, nil)
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"unexpected"})

		if err := command.Execute(); err == nil {
			t.Fatal("Execute() error = nil, want argument error")
		}
	})

	t.Run("table failure", func(t *testing.T) {
		want := errors.New("list failed")
		command := newCommand(func(context.Context) ([]herdr.Agent, error) { return nil, want }, nil)
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})

		if err := command.Execute(); !errors.Is(err, want) {
			t.Fatalf("Execute() error = %v, want %v", err, want)
		}
	})

	t.Run("json failure", func(t *testing.T) {
		want := errors.New("invoke failed")
		command := newCommand(nil, func(context.Context) (json.RawMessage, error) { return nil, want })
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--json"})

		if err := command.Execute(); !errors.Is(err, want) {
			t.Fatalf("Execute() error = %v, want %v", err, want)
		}
	})
}

func TestListHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context) ([]herdr.Agent, error) {
		t.Fatal("list operation called")
		return nil, nil
	}, nil)
	command.SetOut(io.Discard)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
