package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

const placementWorkspacesReply = `{"id":"1","result":{"type":"workspace_list","workspaces":[{"workspace_id":"w1","label":"main"}]}}`
const placementExistingTabsReply = `{"id":"1","result":{"type":"tab_list","tabs":[{"tab_id":"w1:t1","workspace_id":"w1","label":"code"}]}}`
const placementEmptyTabsReply = `{"id":"1","result":{"type":"tab_list","tabs":[]}}`
const placementCreatedTabReply = `{"id":"1","result":{"type":"tab_created","tab":{"tab_id":"w1:t9"},"root_pane":{"pane_id":"w1:p9"}}}`

func targetedSpawnRequest() *protocol.Request {
	return &protocol.Request{
		Model:     "claude-opus-4",
		Workspace: "main",
		Tab:       "code",
	}
}

func serveConcurrentHerdr(t *testing.T, replies map[string]string) *fakeHerdr {
	t.Helper()
	f := &fakeHerdr{t: t, socket: filepath.Join(t.TempDir(), "h.sock")}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	var handlers sync.WaitGroup
	t.Cleanup(func() {
		ln.Close()
		<-done
		handlers.Wait()
	})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				f.handle(conn, replies)
			}()
		}
	}()
	return f
}

func TestTargetedSpawnResolvesLabelsAndReusesTab(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementExistingTabsReply,
		"agent.start":    paneStartedReply,
	})
	d := boundDaemon(t, f)

	resp, err := d.spawn(targetedSpawnRequest())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	start := f.params("agent.start")
	if start["workspace_id"] != "w1" || start["tab_id"] != "w1:t1" || start["focus"] != false {
		t.Fatalf("agent.start target = %+v", start)
	}
	if f.count("tab.create") != 0 {
		t.Fatal("reused tab was recreated")
	}
	agent := d.agents[resp.Name]
	if agent.WorkspaceID != "w1" || agent.WorkspaceLabel != "main" || agent.TabID != "w1:t1" || agent.TabLabel != "code" {
		t.Fatalf("agent placement = %+v", agent)
	}
	placed := findEvent(t, d, evPlaced, resp.Name)
	if placed.WorkspaceID != "w1" || placed.TabID != "w1:t1" {
		t.Fatalf("agent.placed = %+v", placed)
	}

	if _, err := d.stop(&protocol.Request{Name: resp.Name}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if f.count("tab.close") != 0 {
		t.Fatal("reused tab was closed")
	}
}

func TestTargetedSpawnCreatesAndClosesOwnedTab(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     placementCreatedTabReply,
		"agent.start":    paneStartedReply,
	})
	d := boundDaemon(t, f)
	req := targetedSpawnRequest()
	req.Tab = "review"

	resp, err := d.spawn(req)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	create := f.params("tab.create")
	createLabel, _ := create["label"].(string)
	if create["workspace_id"] != "w1" || !strings.HasPrefix(createLabel, "fledge-create-") || create["focus"] != false {
		t.Fatalf("tab.create = %+v", create)
	}
	if rename := f.params("tab.rename"); rename["tab_id"] != "w1:t9" || rename["label"] != "review" {
		t.Fatalf("tab.rename = %+v", rename)
	}
	if f.count("pane.close") != 1 {
		t.Fatalf("initial shell closes = %d, want 1", f.count("pane.close"))
	}
	if _, ok := d.ownedTabs["w1:t9"]; !ok {
		t.Fatal("created tab ownership was not retained")
	}
	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replayed.ownedTabs["w1:t9"]; !ok {
		t.Fatal("created tab ownership did not replay")
	}
	if got := replayed.agents[resp.Name].TabID; got != "w1:t9" {
		t.Fatalf("replayed tab id = %q", got)
	}

	if _, err := d.stop(&protocol.Request{Name: resp.Name}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if f.count("tab.close") != 1 {
		t.Fatalf("tab closes = %d, want 1", f.count("tab.close"))
	}
	if _, ok := d.ownedTabs["w1:t9"]; ok {
		t.Fatal("closed tab ownership remains live")
	}
	if countEvents(t, d, evTabCreated, "") != 1 || countEvents(t, d, evTabClosed, "") != 1 {
		t.Fatalf("tab lifecycle events = %+v", events(t, d))
	}
}

func TestTargetedSpawnValidationAndAmbiguity(t *testing.T) {
	t.Run("both dimensions", func(t *testing.T) {
		d := boundDaemon(t, nil)
		_, err := d.spawn(&protocol.Request{Model: "claude-opus-4", Workspace: "main"})
		if err == nil || !strings.Contains(err.Error(), "provided together") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("pi", func(t *testing.T) {
		d := boundDaemon(t, nil)
		_, err := d.spawn(&protocol.Request{Model: "gpt-x", Workspace: "main", Tab: "code"})
		if err == nil || !strings.Contains(err.Error(), "pi profiles") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("dedicated definition", func(t *testing.T) {
		req := targetedSpawnRequest()
		_, err := validateSpawnPlacement(req, spawnResolution{
			cfg:       agentcfg.Config{Integration: "claude"},
			agent:     "context-planner",
			workspace: &agentcfg.Workspace{Label: "dedicated", Tab: "context"},
		})
		if err == nil || !strings.Contains(err.Error(), "cannot also target") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("split ambiguity", func(t *testing.T) {
		req := targetedSpawnRequest()
		req.Split = "right"
		_, err := validateSpawnPlacement(req, spawnResolution{
			cfg: agentcfg.Config{Integration: "claude"},
		})
		if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("workspace label", func(t *testing.T) {
		f := serveHerdr(t, map[string]string{
			"workspace.list": `{"id":"1","result":{"workspaces":[{"workspace_id":"w1","label":"same"},{"workspace_id":"w2","label":"same"}]}}`,
		})
		d := boundDaemon(t, f)
		req := targetedSpawnRequest()
		req.Workspace = "same"
		if _, err := d.spawn(req); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("tab label", func(t *testing.T) {
		f := serveHerdr(t, map[string]string{
			"workspace.list": placementWorkspacesReply,
			"tab.list": `{"id":"1","result":{"tabs":[
				{"tab_id":"w1:t1","workspace_id":"w1","label":"same"},
				{"tab_id":"w1:t2","workspace_id":"w1","label":"same"}]}}`,
		})
		d := boundDaemon(t, f)
		req := targetedSpawnRequest()
		req.Tab = "same"
		if _, err := d.spawn(req); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing tab id", func(t *testing.T) {
		f := serveHerdr(t, map[string]string{
			"workspace.list": placementWorkspacesReply,
			"tab.list":       placementEmptyTabsReply,
		})
		d := boundDaemon(t, f)
		req := targetedSpawnRequest()
		req.Tab = "w1:t404"
		if _, err := d.spawn(req); err == nil || !strings.Contains(err.Error(), "no tab with id") {
			t.Fatalf("error = %v", err)
		}
		if f.count("tab.create") != 0 {
			t.Fatal("missing tab id was treated as a label")
		}
	})
	t.Run("colon label", func(t *testing.T) {
		f := serveHerdr(t, map[string]string{
			"workspace.list": placementWorkspacesReply,
			"tab.list":       placementEmptyTabsReply,
			"tab.create":     placementCreatedTabReply,
			"agent.start":    paneStartedReply,
		})
		d := boundDaemon(t, f)
		req := targetedSpawnRequest()
		req.Tab = "review:urgent"
		if _, err := d.spawn(req); err != nil {
			t.Fatalf("spawn colon label: %v", err)
		}
		if create := f.params("tab.create"); !strings.HasPrefix(create["label"].(string), "fledge-create-") {
			t.Fatalf("tab.create = %+v", create)
		}
		if rename := f.params("tab.rename"); rename["label"] != "review:urgent" {
			t.Fatalf("tab.rename = %+v", rename)
		}
	})
}

func TestLooksLikeTabIDRecognizesOnlyHerdrSyntax(t *testing.T) {
	for _, selector := range []string{"w1:t2", "w0:t0", "w123:t456"} {
		if !looksLikeTabID(selector) {
			t.Errorf("looksLikeTabID(%q) = false", selector)
		}
	}
	for _, selector := range []string{"review:urgent", "w:t2", "w1:t", "w1:x2", "w1:t2:note", "W1:T2", "prefix:w1:t2"} {
		if looksLikeTabID(selector) {
			t.Errorf("looksLikeTabID(%q) = true", selector)
		}
	}
}

func TestExternalTabCreateRaceRollsBackOnlyFledgeTab(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.create":     placementCreatedTabReply,
		"agent.start":    paneStartedReply,
	})
	f.mu.Lock()
	f.sequences = map[string][]string{
		"tab.list": {
			placementEmptyTabsReply,
			placementEmptyTabsReply,
			`{"id":"1","result":{"type":"tab_list","tabs":[
				{"tab_id":"w1:t8","workspace_id":"w1","label":"review"},
				{"tab_id":"w1:t9","workspace_id":"w1","label":"review"}]}}`,
		},
	}
	f.mu.Unlock()
	d := boundDaemon(t, f)
	req := targetedSpawnRequest()
	req.Tab = "review"

	if _, err := d.spawn(req); err == nil || !strings.Contains(err.Error(), "became ambiguous") {
		t.Fatalf("spawn error = %v", err)
	}
	if f.count("agent.start") != 0 {
		t.Fatal("agent started into an ambiguous tab label")
	}
	if f.count("tab.close") != 1 {
		t.Fatalf("tab.close count = %d, want only Fledge rollback", f.count("tab.close"))
	}
	if close := f.params("tab.close"); close["tab_id"] != "w1:t9" {
		t.Fatalf("tab.close = %+v, want Fledge-created w1:t9", close)
	}
	if countEvents(t, d, evTabCreated, "") != 1 || countEvents(t, d, evTabClosed, "") != 1 {
		t.Fatalf("ambiguous created tab did not complete durable rollback: %+v", events(t, d))
	}
}

func TestConcurrentTargetedSpawnsConvergeOnCreatedTab(t *testing.T) {
	f := serveConcurrentHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     placementCreatedTabReply,
		"agent.start":    paneStartedReply,
	})
	blockCreate := make(chan struct{})
	f.mu.Lock()
	f.blocks = map[string]<-chan struct{}{"tab.create": blockCreate}
	f.mu.Unlock()
	d := boundDaemon(t, f)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	names := make(chan string, 2)
	spawn := func() {
		defer wg.Done()
		req := targetedSpawnRequest()
		req.Tab = "review"
		resp, err := d.spawn(req)
		errs <- err
		names <- resp.Name
	}
	wg.Add(1)
	go spawn()
	waitFor(t, func() bool { return f.count("tab.create") == 1 })
	wg.Add(1)
	go spawn()
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		latch := d.tabCreates["w1\x00review"]
		return latch != nil && len(latch.waiters) == 2
	})
	close(blockCreate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
	}
	if f.count("tab.create") != 1 {
		t.Fatalf("tab.create count = %d, want 1", f.count("tab.create"))
	}

	close(names)
	for name := range names {
		if name == "" {
			t.Fatal("spawn returned no name")
		}
		if _, err := d.stop(&protocol.Request{Name: name}); err != nil {
			t.Fatalf("stop %s: %v", name, err)
		}
	}
	if f.count("tab.close") != 1 {
		t.Fatalf("tab.close count = %d, want 1 after last stop", f.count("tab.close"))
	}
}

func TestTargetedLaunchFailureRollsBackCreatedTab(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     placementCreatedTabReply,
		"agent.start":    `{"id":"1","error":{"code":"failed","message":"nope"}}`,
	})
	d := boundDaemon(t, f)
	req := targetedSpawnRequest()
	req.Tab = "review"

	if _, err := d.spawn(req); err == nil {
		t.Fatal("spawn succeeded")
	}
	if f.count("tab.close") != 1 {
		t.Fatalf("tab.close count = %d, want rollback", f.count("tab.close"))
	}
	if got := agentState(d, "claude-emperor"); got != stateStopped {
		t.Fatalf("failed agent state = %q", got)
	}
}

func TestTargetedTabOwnershipJournalFailureRollsBackBeforeLaunch(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     placementCreatedTabReply,
	})
	d := boundDaemon(t, f)
	// launching and the creation intent are the first two writes through this
	// wrapper; tab.created is the third and must become authoritative before
	// rename or agent.start is allowed.
	d.journal = &failNthJournal{WriteCloser: d.journal, failAt: 3}
	req := targetedSpawnRequest()
	req.Tab = "review"

	if _, err := d.spawn(req); err == nil {
		t.Fatal("spawn succeeded")
	}
	if f.count("agent.start") != 0 {
		t.Fatal("agent launched before tab ownership was journaled")
	}
	if f.count("tab.close") != 1 {
		t.Fatalf("tab.close count = %d, want rollback", f.count("tab.close"))
	}
	if f.count("tab.rename") != 0 {
		t.Fatal("unjournaled tab was renamed before rollback")
	}
}

func TestTargetedTabCreateIntentJournalFailureDoesNotCreate(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
	})
	d := boundDaemon(t, f)
	// launching is first; the pre-create intent is second.
	d.journal = &failNthJournal{WriteCloser: d.journal, failAt: 2}
	req := targetedSpawnRequest()
	req.Tab = "review"

	if _, err := d.spawn(req); err == nil {
		t.Fatal("spawn succeeded")
	}
	if f.count("tab.create") != 0 {
		t.Fatal("tab.create ran before its durable intent")
	}
}

func TestTargetedTabCreateRPCErrorRetainsIntentForReplay(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     `{"id":"1","error":{"code":"failed","message":"uncertain"}}`,
	})
	d := boundDaemon(t, f)
	req := targetedSpawnRequest()
	req.Tab = "review"

	if _, err := d.spawn(req); err == nil {
		t.Fatal("spawn succeeded")
	}
	if len(d.tabCreateIntents) != 1 {
		t.Fatalf("ambiguous RPC outcome intents = %+v", d.tabCreateIntents)
	}
	if countEvents(t, d, evTabCreateIntent, "") != 1 ||
		countEvents(t, d, evTabCreateResolved, "") != 0 {
		t.Fatalf("ambiguous RPC outcome journal = %+v", events(t, d))
	}
}

func TestWorkspaceOwnerStopJournalsNestedPlacedAgents(t *testing.T) {
	f := serveHerdr(t, nil)
	d := boundDaemon(t, f)
	d.mu.Lock()
	d.agents["owner-emperor"] = protocol.Agent{
		Name: "owner-emperor", Integration: "claude", State: stateRunning,
		WorkspaceID: "w9", OwnsWorkspace: true,
	}
	d.agents["nested-emperor"] = protocol.Agent{
		Name: "nested-emperor", Integration: "codex", State: stateRunning,
		WorkspaceID: "w9", TabID: "w9:t2",
	}
	d.order = append(d.order, "owner-emperor", "nested-emperor")
	d.ownedTabs["w9:t2"] = ownedTab{WorkspaceID: "w9", TabID: "w9:t2", Label: "review"}
	if err := d.appendAll(
		event{Event: evRegistered, Name: "owner-emperor"},
		event{Event: evSpawned, Name: "owner-emperor", WorkspaceID: "w9", OwnsWorkspace: true},
		event{Event: evRegistered, Name: "nested-emperor"},
		event{Event: evPlaced, Name: "nested-emperor", WorkspaceID: "w9", TabID: "w9:t2"},
		event{Event: evSpawned, Name: "nested-emperor", WorkspaceID: "w9", TabID: "w9:t2"},
		event{Event: evTabCreated, WorkspaceID: "w9", TabID: "w9:t2", TabLabel: "review"},
	); err != nil {
		d.mu.Unlock()
		t.Fatal(err)
	}
	d.mu.Unlock()

	if _, err := d.stop(&protocol.Request{Name: "owner-emperor"}); err != nil {
		t.Fatalf("stop owner: %v", err)
	}
	if f.count("workspace.close") != 1 || f.count("pane.close") != 0 || f.count("tab.close") != 0 {
		t.Fatalf("Herdr methods = %v", f.methods())
	}
	if agentState(d, "owner-emperor") != stateStopped || agentState(d, "nested-emperor") != stateStopped {
		t.Fatalf("agents = %+v", d.agents)
	}
	stopped := findEvent(t, d, evStopped, "nested-emperor")
	if stopped.Reason != "workspace owner stopped" {
		t.Fatalf("nested stop = %+v", stopped)
	}
	if _, ok := d.ownedTabs["w9:t2"]; ok {
		t.Fatal("workspace-close tab ownership remains")
	}
}

func TestRecoverOwnedTabsClosesOnlyUnreferencedAuthority(t *testing.T) {
	f := serveHerdr(t, nil)
	d := boundDaemon(t, f)
	d.mu.Lock()
	d.ownedTabs["w1:t8"] = ownedTab{WorkspaceID: "w1", TabID: "w1:t8", Label: "stale"}
	d.ownedTabs["w1:t9"] = ownedTab{WorkspaceID: "w1", TabID: "w1:t9", Label: "live"}
	d.agents["worker-emperor"] = protocol.Agent{Name: "worker-emperor", State: stateRunning, TabID: "w1:t9"}
	d.mu.Unlock()

	if err := d.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if f.count("tab.close") != 1 {
		t.Fatalf("tab.close count = %d, want stale tab only", f.count("tab.close"))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.ownedTabs["w1:t8"]; ok {
		t.Fatal("stale ownership remains")
	}
	if _, ok := d.ownedTabs["w1:t9"]; !ok {
		t.Fatal("live ownership was discarded")
	}
}

func TestRecoverTabCreateIntentBeforeCreateLeavesExternalSameLabel(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"tab.list": `{"id":"1","result":{"tabs":[
			{"tab_id":"w1:t8","workspace_id":"w1","label":"review"}]}}`,
	})
	d := boundDaemon(t, f)
	intent := event{
		Event:       evTabCreateIntent,
		IntentID:    "intent-before",
		WorkspaceID: "w1",
		TabLabel:    "review",
		CreateLabel: "fledge-create-intent-before",
		Cwd:         d.root,
	}
	if err := d.append(intent); err != nil {
		t.Fatal(err)
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })
	restarted.session = d.session
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if f.count("tab.close") != 0 {
		t.Fatal("recovery closed an external requested-label tab")
	}
	if len(restarted.tabCreateIntents) != 0 || !hasEvent(t, restarted, evTabCreateResolved, "") {
		t.Fatalf("creation intent did not resolve: %+v", events(t, restarted))
	}
}

func TestRecoverTabCreateIntentAfterCreateRollsBackAndIsIdempotent(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"tab.list": `{"id":"1","result":{"tabs":[
			{"tab_id":"w1:t9","workspace_id":"w1","label":"fledge-create-intent-after"}]}}`,
	})
	d := boundDaemon(t, f)
	if err := d.append(event{
		Event:       evTabCreateIntent,
		IntentID:    "intent-after",
		WorkspaceID: "w1",
		TabLabel:    "review",
		CreateLabel: "fledge-create-intent-after",
		Cwd:         d.root,
	}); err != nil {
		t.Fatal(err)
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	restarted.session = d.session
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if close := f.params("tab.close"); close["tab_id"] != "w1:t9" {
		t.Fatalf("tab.close = %+v", close)
	}
	if len(restarted.tabCreateIntents) != 0 || len(restarted.ownedTabs) != 0 {
		t.Fatalf("recovered state: intents=%+v owned=%+v", restarted.tabCreateIntents, restarted.ownedTabs)
	}
	if countEvents(t, restarted, evTabCreated, "") != 1 ||
		countEvents(t, restarted, evTabClosing, "") != 1 ||
		countEvents(t, restarted, evTabClosed, "") != 1 {
		t.Fatalf("rollback lifecycle = %+v", events(t, restarted))
	}
	restarted.Close()

	again, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("second restart: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	again.session = d.session
	if err := again.recoverOwnedTabs(); err != nil {
		t.Fatalf("second recoverOwnedTabs: %v", err)
	}
	if f.count("tab.list") != 1 || f.count("tab.close") != 1 {
		t.Fatalf("idempotent replay methods = %v", f.methods())
	}
}

func TestRecoverTabCreateIntentJournalFailureDoesNotClose(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"tab.list": `{"id":"1","result":{"tabs":[
			{"tab_id":"w1:t9","workspace_id":"w1","label":"fledge-create-intent-fail"}]}}`,
	})
	d := boundDaemon(t, f)
	if err := d.append(event{
		Event:       evTabCreateIntent,
		IntentID:    "intent-fail",
		WorkspaceID: "w1",
		TabLabel:    "review",
		CreateLabel: "fledge-create-intent-fail",
	}); err != nil {
		t.Fatal(err)
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })
	restarted.session = d.session
	base := restarted.journal
	restarted.journal = &failNthJournal{WriteCloser: base, failAt: 1}
	if err := restarted.recoverOwnedTabs(); err == nil || !strings.Contains(err.Error(), "injected journal failure") {
		t.Fatalf("recover error = %v", err)
	}
	if f.count("tab.close") != 0 {
		t.Fatal("recovery closed tab before ownership was durable")
	}
	if len(restarted.tabCreateIntents) != 1 {
		t.Fatal("failed recovery discarded its durable intent")
	}

	restarted.journal = base
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("retry recoverOwnedTabs: %v", err)
	}
	if f.count("tab.close") != 1 || len(restarted.tabCreateIntents) != 0 {
		t.Fatalf("retry did not converge: methods=%v intents=%+v", f.methods(), restarted.tabCreateIntents)
	}
}

func TestRecoverAmbiguousTemporaryLabelPreservesAllTabs(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"tab.list": `{"id":"1","result":{"tabs":[
			{"tab_id":"w1:t8","workspace_id":"w1","label":"fledge-create-duplicate"},
			{"tab_id":"w1:t9","workspace_id":"w1","label":"fledge-create-duplicate"}]}}`,
	})
	d := boundDaemon(t, f)
	if err := d.append(event{
		Event:       evTabCreateIntent,
		IntentID:    "duplicate",
		WorkspaceID: "w1",
		TabLabel:    "review",
		CreateLabel: "fledge-create-duplicate",
	}); err != nil {
		t.Fatal(err)
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })
	restarted.session = d.session
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if f.count("tab.close") != 0 {
		t.Fatal("ambiguous attribution closed a tab")
	}
	if len(restarted.tabCreateIntents) != 0 {
		t.Fatal("ambiguous intent did not converge")
	}
}

func TestRecoverCompletedCreateBeforeRenameClosesExactTabID(t *testing.T) {
	f := serveHerdr(t, nil)
	d := boundDaemon(t, f)
	if err := d.appendAll(
		event{
			Event:       evTabCreateIntent,
			IntentID:    "completed",
			WorkspaceID: "w1",
			TabLabel:    "review",
			CreateLabel: "fledge-create-completed",
		},
		event{
			Event:       evTabCreated,
			IntentID:    "completed",
			WorkspaceID: "w1",
			TabID:       "w1:t9",
			TabLabel:    "review",
		},
	); err != nil {
		t.Fatal(err)
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })
	restarted.session = d.session
	if len(restarted.tabCreateIntents) != 0 {
		t.Fatal("tab.created did not resolve its creation intent on replay")
	}
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if close := f.params("tab.close"); close["tab_id"] != "w1:t9" {
		t.Fatalf("tab.close = %+v", close)
	}
}

func TestTabCloseCompletionFailureReplaysAndConverges(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     placementCreatedTabReply,
		"agent.start":    paneStartedReply,
	})
	f.mu.Lock()
	f.sequences = map[string][]string{
		"tab.close": {
			`{"id":"1","result":{}}`,
			`{"id":"1","error":{"code":"unknown_tab","message":"no such tab"}}`,
		},
	}
	f.mu.Unlock()
	d := boundDaemon(t, f)
	req := targetedSpawnRequest()
	req.Tab = "review"
	resp, err := d.spawn(req)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	baseJournal := d.journal
	d.journal = &failNthJournal{WriteCloser: baseJournal, failAt: 3}
	if _, err := d.stop(&protocol.Request{Name: resp.Name}); err == nil || !strings.Contains(err.Error(), "injected journal failure") {
		t.Fatalf("stop error = %v", err)
	}
	if !hasEvent(t, d, evTabClosing, "") || hasEvent(t, d, evTabClosed, "") {
		t.Fatalf("tab close journal before restart = %+v", events(t, d))
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })
	restarted.session = d.session
	if _, ok := restarted.tabClosures["w1:t9"]; !ok {
		t.Fatal("pending tab closure did not replay")
	}
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if f.count("tab.close") != 2 {
		t.Fatalf("tab.close count = %d, want original plus idempotent retry", f.count("tab.close"))
	}
	if _, ok := restarted.ownedTabs["w1:t9"]; ok {
		t.Fatal("recovered tab ownership remains")
	}
	if !hasEvent(t, restarted, evTabClosed, "") {
		t.Fatal("recovered tab closure was not completed")
	}
}

func TestTabCloseIntentFailureDoesNotMutateTab(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.list": placementWorkspacesReply,
		"tab.list":       placementEmptyTabsReply,
		"tab.create":     placementCreatedTabReply,
		"agent.start":    paneStartedReply,
	})
	d := boundDaemon(t, f)
	req := targetedSpawnRequest()
	req.Tab = "review"
	resp, err := d.spawn(req)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	d.journal = &failNthJournal{WriteCloser: d.journal, failAt: 2}
	if _, err := d.stop(&protocol.Request{Name: resp.Name}); err == nil {
		t.Fatal("stop succeeded despite close-intent journal failure")
	}
	if f.count("tab.close") != 0 {
		t.Fatal("tab was closed before its durable intent")
	}
	if _, ok := d.ownedTabs["w1:t9"]; !ok {
		t.Fatal("tab ownership was discarded after intent failure")
	}
}

func TestWorkspaceCloseCompletionFailureReplaysCascade(t *testing.T) {
	f := serveHerdr(t, nil)
	f.mu.Lock()
	f.sequences = map[string][]string{
		"workspace.close": {
			`{"id":"1","result":{}}`,
			`{"id":"1","error":{"code":"unknown_workspace","message":"no such workspace"}}`,
		},
	}
	f.mu.Unlock()
	d := boundDaemon(t, f)
	d.mu.Lock()
	d.agents["owner-emperor"] = protocol.Agent{
		Name: "owner-emperor", Integration: "claude", State: stateRunning,
		WorkspaceID: "w9", OwnsWorkspace: true,
	}
	d.agents["nested-emperor"] = protocol.Agent{
		Name: "nested-emperor", Integration: "codex", State: stateRunning,
		WorkspaceID: "w9", TabID: "w9:t2",
	}
	d.agents["sender-emperor"] = protocol.Agent{
		Name: "sender-emperor", PID: os.Getpid(), State: stateRunning,
	}
	d.order = append(d.order, "owner-emperor", "nested-emperor", "sender-emperor")
	d.ownedTabs["w9:t2"] = ownedTab{WorkspaceID: "w9", TabID: "w9:t2", Label: "review"}
	if err := d.appendAll(
		event{Event: evRegistered, Name: "owner-emperor"},
		event{Event: evSpawned, Name: "owner-emperor", WorkspaceID: "w9", OwnsWorkspace: true},
		event{Event: evRegistered, Name: "nested-emperor"},
		event{Event: evPlaced, Name: "nested-emperor", WorkspaceID: "w9", TabID: "w9:t2"},
		event{Event: evSpawned, Name: "nested-emperor", WorkspaceID: "w9", TabID: "w9:t2"},
		event{Event: evRegistered, Name: "sender-emperor", PID: os.Getpid()},
		event{Event: evTabCreated, WorkspaceID: "w9", TabID: "w9:t2", TabLabel: "review"},
		event{Event: evSent, ID: "owner-message", From: "sender-emperor", To: "owner-emperor", Body: "pending"},
		event{Event: evSent, ID: "nested-message", From: "sender-emperor", To: "nested-emperor", Body: "pending"},
	); err != nil {
		d.mu.Unlock()
		t.Fatal(err)
	}
	d.inboxWake = func(context.Context, inboxWakeTarget, inboxWakeMetadata) error {
		t.Fatal("closing workspace identity received an inbox wake")
		return nil
	}
	d.inboxNotifyArmed["owner-emperor"] = true
	d.inboxNotifyArmed["nested-emperor"] = true
	for _, msg := range []protocol.Message{
		{ID: "owner-message", From: "sender-emperor", To: "owner-emperor", Body: "pending"},
		{ID: "nested-message", From: "sender-emperor", To: "nested-emperor", Body: "pending"},
	} {
		d.messages[msg.ID] = msg
		d.messageOrder = append(d.messageOrder, msg.ID)
		d.pending = append(d.pending, msg)
	}
	d.mu.Unlock()

	d.journal = &failNthJournal{WriteCloser: d.journal, failAt: 2}
	if _, err := d.stop(&protocol.Request{Name: "owner-emperor"}); err == nil {
		t.Fatal("workspace stop succeeded despite completion journal failure")
	}
	if agentState(d, "owner-emperor") != stateRunning || agentState(d, "nested-emperor") != stateRunning {
		t.Fatalf("live state moved ahead of journal: %+v", d.agents)
	}
	if !hasEvent(t, d, evWorkspaceClosing, "owner-emperor") || hasEvent(t, d, evWorkspaceClosed, "") {
		t.Fatalf("workspace journal before restart = %+v", events(t, d))
	}
	for _, name := range []string{"owner-emperor", "nested-emperor"} {
		messageID := strings.TrimSuffix(name, "-emperor") + "-message"
		checks := map[string]func() error{
			"send": func() error {
				_, err := d.send(&protocol.Request{From: name, To: "sender-emperor", Body: "stale"})
				return err
			},
			"recipient": func() error {
				_, err := d.send(&protocol.Request{From: "sender-emperor", To: name, Body: "stale"})
				return err
			},
			"wait": func() error {
				_, err := d.wait(&protocol.Request{As: name}, nil)
				return err
			},
			"inbox": func() error {
				_, err := d.inbox(&protocol.Request{As: name})
				return err
			},
			"reply": func() error {
				_, err := d.reply(&protocol.Request{From: name, ID: messageID, Body: "stale"})
				return err
			},
		}
		for operation, check := range checks {
			if err := check(); err == nil || !strings.Contains(err.Error(), "closing workspace") {
				t.Errorf("%s %s error = %v, want closing workspace rejection", name, operation, err)
			}
		}
	}
	d.mu.Lock()
	for _, name := range []string{"owner-emperor", "nested-emperor"} {
		if d.inboxNotifyArmed[name] {
			t.Errorf("%s inbox notification was rearmed", name)
		}
		if d.inboxNotifyTasks[name] != nil {
			t.Errorf("%s inbox wake was queued", name)
		}
		if d.shouldNotifyInboxLocked(protocol.Message{ID: "new-" + name, To: name}) {
			t.Errorf("%s remained eligible for inbox notification", name)
		}
	}
	d.mu.Unlock()
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })
	restarted.session = d.session
	if err := restarted.recoverOwnedTabs(); err != nil {
		t.Fatalf("recoverOwnedTabs: %v", err)
	}
	if f.count("workspace.close") != 2 {
		t.Fatalf("workspace.close count = %d, want original plus idempotent retry", f.count("workspace.close"))
	}
	if agentState(restarted, "owner-emperor") != stateStopped || agentState(restarted, "nested-emperor") != stateStopped {
		t.Fatalf("recovered agents = %+v", restarted.agents)
	}
	if _, ok := restarted.ownedTabs["w9:t2"]; ok {
		t.Fatal("cascade-owned tab remains after recovery")
	}
	if stopped := findEvent(t, restarted, evStopped, "nested-emperor"); stopped.Reason != "workspace owner stopped" {
		t.Fatalf("nested stop = %+v", stopped)
	}
	if !hasEvent(t, restarted, evWorkspaceClosed, "") {
		t.Fatal("workspace closure did not complete")
	}
}

func TestResolveTabIDWinsOverDuplicateLabel(t *testing.T) {
	tabs := []herdrwire.Tab{
		{TabID: "w1:t1", Label: "w1:t2"},
		{TabID: "w1:t2", Label: "same"},
	}
	tab, found, err := resolveTab(tabs, "w1:t2")
	if err != nil || !found || tab.TabID != "w1:t2" {
		t.Fatalf("resolveTab = %+v, %v, %v", tab, found, err)
	}
}

func TestResolveWorkspaceIDWinsOverMatchingLabel(t *testing.T) {
	workspaces := []herdrwire.Workspace{
		{WorkspaceID: "w1", Label: "w2"},
		{WorkspaceID: "w2", Label: "main"},
	}
	workspace, err := resolveWorkspace(workspaces, "w2")
	if err != nil || workspace.WorkspaceID != "w2" {
		t.Fatalf("resolveWorkspace = %+v, %v", workspace, err)
	}
}
