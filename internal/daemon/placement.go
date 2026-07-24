package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

// resolvedPlacement is an exact Herdr address. Requested labels never reach
// agent.start: resolution happens first so focus cannot influence placement.
type resolvedPlacement struct {
	WorkspaceID    string
	WorkspaceLabel string
	TabID          string
	TabLabel       string
}

// ownedTab is a tab Fledge created for an absent label. RootPaneID is live-only
// setup state; the durable fields are reconstructed from tab.created/tab.closed.
type ownedTab struct {
	WorkspaceID string
	TabID       string
	Label       string
	RootPaneID  string
}

type tabCreateLatch struct {
	done    chan struct{}
	waiters map[string]struct{}
	target  resolvedPlacement
	err     error
}

type tabShellLatch struct {
	done chan struct{}
	err  error
}

type closeLatch struct {
	done chan struct{}
	err  error
}

func (d *Daemon) acquirePlacement(name, cwd, workspaceSelector, tabSelector string) (resolvedPlacement, error) {
	workspaces, err := herdrwire.WorkspaceList(d.session.SocketPath)
	if err != nil {
		return resolvedPlacement{}, fmt.Errorf("list workspaces for placement: %w", err)
	}
	workspace, err := resolveWorkspace(workspaces, workspaceSelector)
	if err != nil {
		return resolvedPlacement{}, err
	}

	d.mu.Lock()
	if d.closingWorkspaces[workspace.WorkspaceID] {
		d.mu.Unlock()
		return resolvedPlacement{}, fmt.Errorf("workspace %q is closing", workspaceSelector)
	}
	d.mu.Unlock()

	tabs, err := herdrwire.TabList(d.session.SocketPath, workspace.WorkspaceID)
	if err != nil {
		return resolvedPlacement{}, fmt.Errorf("list tabs in workspace %s: %w", workspace.WorkspaceID, err)
	}
	tab, found, err := resolveTab(tabs, tabSelector)
	if err != nil {
		return resolvedPlacement{}, err
	}
	if found {
		target := resolvedPlacement{
			WorkspaceID:    workspace.WorkspaceID,
			WorkspaceLabel: workspace.Label,
			TabID:          tab.TabID,
			TabLabel:       tab.Label,
		}
		if err := d.recordPlacement(name, target); err != nil {
			return resolvedPlacement{}, err
		}
		return target, nil
	}

	key := workspace.WorkspaceID + "\x00" + tabSelector
	d.mu.Lock()
	if d.closingWorkspaces[workspace.WorkspaceID] {
		d.mu.Unlock()
		return resolvedPlacement{}, fmt.Errorf("workspace %q is closing", workspaceSelector)
	}
	if latch := d.tabCreates[key]; latch != nil {
		latch.waiters[name] = struct{}{}
		d.mu.Unlock()
		<-latch.done
		if latch.err != nil {
			return resolvedPlacement{}, latch.err
		}
		if err := d.journalPlacement(name, latch.target); err != nil {
			_ = d.cleanupOwnedTab(latch.target.TabID)
			return resolvedPlacement{}, err
		}
		return latch.target, nil
	}
	latch := &tabCreateLatch{
		done:    make(chan struct{}),
		waiters: map[string]struct{}{name: {}},
	}
	d.tabCreates[key] = latch
	d.mu.Unlock()

	target, createErr := d.createOrReusePlacement(workspace, tabSelector, cwd)

	d.mu.Lock()
	if createErr == nil && d.closingWorkspaces[workspace.WorkspaceID] {
		createErr = fmt.Errorf("workspace %s is closing", workspace.WorkspaceID)
	}
	if createErr == nil {
		for waiter := range latch.waiters {
			d.associatePlacementLocked(waiter, target)
		}
	}
	latch.target, latch.err = target, createErr
	delete(d.tabCreates, key)
	close(latch.done)
	d.mu.Unlock()

	if createErr != nil {
		if target.TabID != "" {
			_ = d.cleanupOwnedTab(target.TabID)
		}
		return resolvedPlacement{}, createErr
	}
	if err := d.journalPlacement(name, target); err != nil {
		_ = d.cleanupOwnedTab(target.TabID)
		return resolvedPlacement{}, err
	}
	return target, nil
}

func resolveWorkspace(workspaces []herdrwire.Workspace, selector string) (herdrwire.Workspace, error) {
	for _, workspace := range workspaces {
		if workspace.WorkspaceID == selector {
			return workspace, nil
		}
	}
	var matches []herdrwire.Workspace
	for _, workspace := range workspaces {
		if workspace.Label == selector {
			matches = append(matches, workspace)
		}
	}
	switch len(matches) {
	case 0:
		return herdrwire.Workspace{}, fmt.Errorf("no workspace with id or label %q", selector)
	case 1:
		return matches[0], nil
	default:
		return herdrwire.Workspace{}, fmt.Errorf("workspace label %q is ambiguous: %d workspaces match", selector, len(matches))
	}
}

func resolveTab(tabs []herdrwire.Tab, selector string) (herdrwire.Tab, bool, error) {
	for _, tab := range tabs {
		if tab.TabID == selector {
			return tab, true, nil
		}
	}
	var matches []herdrwire.Tab
	for _, tab := range tabs {
		if tab.Label == selector {
			matches = append(matches, tab)
		}
	}
	switch len(matches) {
	case 0:
		if looksLikeTabID(selector) {
			return herdrwire.Tab{}, false, fmt.Errorf("no tab with id %q", selector)
		}
		return herdrwire.Tab{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return herdrwire.Tab{}, false, fmt.Errorf("tab label %q is ambiguous: %d tabs match", selector, len(matches))
	}
}

// Herdr protocol-16 tab ids are workspace-qualified (for example w1:t2).
func looksLikeTabID(selector string) bool {
	if len(selector) < 5 || selector[0] != 'w' {
		return false
	}
	i := 1
	for i < len(selector) && selector[i] >= '0' && selector[i] <= '9' {
		i++
	}
	if i == 1 || i+2 >= len(selector) || selector[i] != ':' || selector[i+1] != 't' {
		return false
	}
	i += 2
	start := i
	for i < len(selector) && selector[i] >= '0' && selector[i] <= '9' {
		i++
	}
	return i > start && i == len(selector)
}

func (d *Daemon) createOrReusePlacement(workspace herdrwire.Workspace, label, cwd string) (resolvedPlacement, error) {
	// Recheck after winning the latch. This closes the race with a tab created
	// outside Fledge between the first list and latch acquisition.
	tabs, err := herdrwire.TabList(d.session.SocketPath, workspace.WorkspaceID)
	if err != nil {
		return resolvedPlacement{}, fmt.Errorf("relist tabs in workspace %s: %w", workspace.WorkspaceID, err)
	}
	if tab, found, err := resolveTab(tabs, label); err != nil {
		return resolvedPlacement{}, err
	} else if found {
		return resolvedPlacement{
			WorkspaceID:    workspace.WorkspaceID,
			WorkspaceLabel: workspace.Label,
			TabID:          tab.TabID,
			TabLabel:       tab.Label,
		}, nil
	}

	intent, err := d.beginTabCreate(workspace.WorkspaceID, label, cwd)
	if err != nil {
		return resolvedPlacement{}, err
	}
	created, err := herdrwire.TabCreate(d.session.SocketPath, workspace.WorkspaceID, cwd, intent.CreateLabel)
	if err != nil {
		// An RPC error is not proof the server did not apply the request, and
		// an immediate empty inventory could still race delayed application.
		// Keep the intent durable for startup recovery instead of guessing.
		return resolvedPlacement{}, fmt.Errorf("create tab %q in workspace %s: %w", label, workspace.WorkspaceID, err)
	}
	if created.TabID == "" || created.RootPaneID == "" {
		cause := fmt.Errorf("create tab %q in workspace %s: Herdr returned incomplete tab ids", label, workspace.WorkspaceID)
		if created.TabID != "" {
			if closeErr := herdrwire.TabClose(d.session.SocketPath, created.TabID); closeErr != nil {
				cause = fmt.Errorf("%w (closing tab %s also failed: %v)", cause, created.TabID, closeErr)
			} else if resolveErr := d.resolveTabCreateIntent(intent.IntentID); resolveErr != nil {
				cause = fmt.Errorf("%w (resolving creation intent also failed: %v)", cause, resolveErr)
			}
		}
		return resolvedPlacement{}, cause
	}
	target := resolvedPlacement{
		WorkspaceID:    workspace.WorkspaceID,
		WorkspaceLabel: workspace.Label,
		TabID:          created.TabID,
		TabLabel:       label,
	}

	d.mu.Lock()
	err = d.append(event{
		Event:       evTabCreated,
		WorkspaceID: workspace.WorkspaceID,
		TabID:       created.TabID,
		TabLabel:    label,
		IntentID:    intent.IntentID,
	})
	if err == nil {
		delete(d.tabCreateIntents, intent.IntentID)
		d.ownedTabs[created.TabID] = ownedTab{
			WorkspaceID: workspace.WorkspaceID,
			TabID:       created.TabID,
			Label:       label,
			RootPaneID:  created.RootPaneID,
		}
	}
	d.mu.Unlock()
	if err != nil {
		if closeErr := herdrwire.TabClose(d.session.SocketPath, created.TabID); closeErr != nil {
			err = fmt.Errorf("%w (closing unjournaled tab %s also failed: %v)", err, created.TabID, closeErr)
		} else if resolveErr := d.resolveTabCreateIntent(intent.IntentID); resolveErr != nil {
			err = fmt.Errorf("%w (resolving creation intent also failed: %v)", err, resolveErr)
		}
		return resolvedPlacement{}, err
	}

	// The unique temporary label makes an incomplete tab.create attributable
	// on replay. Only after the returned id is durable may the requested label
	// become visible and participate in normal same-label ambiguity checks.
	if err := herdrwire.TabRename(d.session.SocketPath, created.TabID, label); err != nil {
		cause := fmt.Errorf("rename created tab %s to %q: %w", created.TabID, label, err)
		if closeErr := d.cleanupOwnedTab(created.TabID); closeErr != nil {
			cause = fmt.Errorf("%w (closing created tab %s also failed: %v)", cause, created.TabID, closeErr)
		}
		return resolvedPlacement{}, cause
	}

	// An external creator can race between the final pre-create list and the
	// rename. Re-list afterward and refuse a duplicate label, closing only the
	// tab whose id Herdr returned to this Fledge call.
	tabs, err = herdrwire.TabList(d.session.SocketPath, workspace.WorkspaceID)
	if err != nil {
		cause := fmt.Errorf("relist tabs after creating %q in workspace %s: %w", label, workspace.WorkspaceID, err)
		if closeErr := d.cleanupOwnedTab(created.TabID); closeErr != nil {
			cause = fmt.Errorf("%w (closing created tab %s also failed: %v)", cause, created.TabID, closeErr)
		}
		return resolvedPlacement{}, cause
	}
	matches := 0
	for _, tab := range tabs {
		if tab.Label == label {
			matches++
		}
	}
	if matches > 1 {
		cause := fmt.Errorf("tab label %q became ambiguous during creation: %d tabs match", label, matches)
		if closeErr := d.cleanupOwnedTab(created.TabID); closeErr != nil {
			cause = fmt.Errorf("%w (closing created tab %s also failed: %v)", cause, created.TabID, closeErr)
		}
		return resolvedPlacement{}, cause
	}
	return target, nil
}

func (d *Daemon) beginTabCreate(workspaceID, label, cwd string) (pendingTabCreate, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return pendingTabCreate{}, fmt.Errorf("prepare tab creation intent: %w", err)
	}
	id := hex.EncodeToString(raw[:])
	intent := pendingTabCreate{
		IntentID:    id,
		WorkspaceID: workspaceID,
		TabLabel:    label,
		CreateLabel: "fledge-create-" + id,
		Cwd:         cwd,
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.append(event{
		Event:       evTabCreateIntent,
		IntentID:    intent.IntentID,
		WorkspaceID: intent.WorkspaceID,
		TabLabel:    intent.TabLabel,
		CreateLabel: intent.CreateLabel,
		Cwd:         intent.Cwd,
	}); err != nil {
		return pendingTabCreate{}, err
	}
	d.tabCreateIntents[intent.IntentID] = intent
	return intent, nil
}

func (d *Daemon) resolveTabCreateIntent(intentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.tabCreateIntents[intentID]; !ok {
		return nil
	}
	if err := d.append(event{Event: evTabCreateResolved, IntentID: intentID}); err != nil {
		return err
	}
	delete(d.tabCreateIntents, intentID)
	return nil
}

func (d *Daemon) recordPlacement(name string, target resolvedPlacement) error {
	d.mu.Lock()
	if d.closingWorkspaces[target.WorkspaceID] {
		d.mu.Unlock()
		return fmt.Errorf("workspace %s is closing", target.WorkspaceID)
	}
	if d.closingTabs[target.TabID] {
		d.mu.Unlock()
		return fmt.Errorf("tab %s is closing", target.TabID)
	}
	d.associatePlacementLocked(name, target)
	if err := d.append(placementEvent(name, target)); err != nil {
		d.clearPlacementLocked(name, target.TabID)
		d.mu.Unlock()
		_ = d.cleanupOwnedTab(target.TabID)
		return err
	}
	d.mu.Unlock()
	return nil
}

func (d *Daemon) journalPlacement(name string, target resolvedPlacement) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.append(placementEvent(name, target)); err != nil {
		d.clearPlacementLocked(name, target.TabID)
		return err
	}
	return nil
}

func placementEvent(name string, target resolvedPlacement) event {
	return event{
		Event:          evPlaced,
		Name:           name,
		WorkspaceID:    target.WorkspaceID,
		WorkspaceLabel: target.WorkspaceLabel,
		TabID:          target.TabID,
		TabLabel:       target.TabLabel,
	}
}

func (d *Daemon) associatePlacementLocked(name string, target resolvedPlacement) {
	agent, ok := d.agents[name]
	if !ok {
		return
	}
	agent.WorkspaceID = target.WorkspaceID
	agent.WorkspaceLabel = target.WorkspaceLabel
	agent.TabID = target.TabID
	agent.TabLabel = target.TabLabel
	agent.OwnsWorkspace = false
	d.agents[name] = agent
}

func (d *Daemon) clearPlacementLocked(name, tabID string) {
	agent, ok := d.agents[name]
	if !ok || agent.TabID != tabID {
		return
	}
	agent.WorkspaceID = ""
	agent.WorkspaceLabel = ""
	agent.TabID = ""
	agent.TabLabel = ""
	d.agents[name] = agent
}

// finishOwnedTabSetup removes the initial shell Herdr creates with an
// ephemeral tab. Concurrent launches share one close result.
func (d *Daemon) finishOwnedTabSetup(tabID string) error {
	d.mu.Lock()
	owned, ok := d.ownedTabs[tabID]
	if !ok || owned.RootPaneID == "" {
		d.mu.Unlock()
		return nil
	}
	if latch := d.tabShells[tabID]; latch != nil {
		d.mu.Unlock()
		<-latch.done
		return latch.err
	}
	latch := &tabShellLatch{done: make(chan struct{})}
	d.tabShells[tabID] = latch
	rootPaneID := owned.RootPaneID
	d.mu.Unlock()

	err := herdrwire.PaneClose(d.session.SocketPath, rootPaneID)

	d.mu.Lock()
	if err == nil {
		owned = d.ownedTabs[tabID]
		owned.RootPaneID = ""
		d.ownedTabs[tabID] = owned
	}
	latch.err = err
	delete(d.tabShells, tabID)
	close(latch.done)
	d.mu.Unlock()
	if err != nil {
		return fmt.Errorf("remove initial shell from tab %s: %w", tabID, err)
	}
	return nil
}

func (d *Daemon) cleanupOwnedTab(tabID string) error {
	if tabID == "" {
		return nil
	}
	d.mu.Lock()
	owned, ok := d.ownedTabs[tabID]
	if !ok {
		d.mu.Unlock()
		return nil
	}
	if run := d.tabCloseRuns[tabID]; run != nil {
		d.mu.Unlock()
		<-run.done
		return run.err
	}
	closure, pending := d.tabClosures[tabID]
	if !pending && d.hasActivePlacementLocked(tabID) {
		d.mu.Unlock()
		return nil
	}
	if !pending {
		closure = tabRecord{
			WorkspaceID: owned.WorkspaceID,
			TabID:       owned.TabID,
			TabLabel:    owned.Label,
		}
		if err := d.append(event{
			Event:       evTabClosing,
			WorkspaceID: closure.WorkspaceID,
			TabID:       closure.TabID,
			TabLabel:    closure.TabLabel,
		}); err != nil {
			d.mu.Unlock()
			return err
		}
		d.tabClosures[tabID] = closure
	}
	run := &closeLatch{done: make(chan struct{})}
	d.tabCloseRuns[tabID] = run
	d.closingTabs[tabID] = true
	d.mu.Unlock()

	err := d.closeTabIdempotent(closure)

	d.mu.Lock()
	if err == nil {
		err = d.append(event{
			Event:       evTabClosed,
			WorkspaceID: closure.WorkspaceID,
			TabID:       closure.TabID,
			TabLabel:    closure.TabLabel,
		})
	}
	if err == nil {
		delete(d.ownedTabs, tabID)
		delete(d.tabClosures, tabID)
		delete(d.closingTabs, tabID)
	}
	run.err = err
	delete(d.tabCloseRuns, tabID)
	close(run.done)
	d.mu.Unlock()
	return err
}

// closeTabIdempotent distinguishes a failed close from a close that already
// happened before a crash or completion-journal failure by checking Herdr's
// current inventory. Herdr protocol 16 does not promise a stable "not found"
// error code, so inventory is the portable authority.
func (d *Daemon) closeTabIdempotent(tab tabRecord) error {
	err := herdrwire.TabClose(d.session.SocketPath, tab.TabID)
	if err == nil {
		return nil
	}
	workspaces, listErr := herdrwire.WorkspaceList(d.session.SocketPath)
	if listErr != nil {
		return fmt.Errorf("close Fledge-created tab %s: %w (verify workspaces: %v)", tab.TabID, err, listErr)
	}
	workspaceExists := false
	for _, workspace := range workspaces {
		if workspace.WorkspaceID == tab.WorkspaceID {
			workspaceExists = true
			break
		}
	}
	if !workspaceExists {
		return nil
	}
	tabs, listErr := herdrwire.TabList(d.session.SocketPath, tab.WorkspaceID)
	if listErr != nil {
		return fmt.Errorf("close Fledge-created tab %s: %w (verify tabs: %v)", tab.TabID, err, listErr)
	}
	for _, live := range tabs {
		if live.TabID == tab.TabID {
			return fmt.Errorf("close Fledge-created tab %s: %w", tab.TabID, err)
		}
	}
	return nil
}

func (d *Daemon) hasActivePlacementLocked(tabID string) bool {
	for _, agent := range d.agents {
		if agent.TabID == tabID && agent.State != stateStopped && agent.State != stateOrphaned {
			return true
		}
	}
	return false
}

// recoverOwnedTabs closes crash-left ephemeral tabs that no replayed live
// placement still uses. Active ownership stays authoritative for later stops.
func (d *Daemon) recoverOwnedTabs() error {
	d.mu.Lock()
	var workspaces []string
	for workspaceID := range d.workspaceClosures {
		workspaces = append(workspaces, workspaceID)
	}
	d.mu.Unlock()
	sort.Strings(workspaces)
	for _, workspaceID := range workspaces {
		d.mu.Lock()
		closure := d.workspaceClosures[workspaceID]
		d.mu.Unlock()
		if err := d.finishWorkspaceClosure(closure); err != nil {
			return fmt.Errorf("recover workspace closure: %w", err)
		}
	}

	d.mu.Lock()
	var pendingTabs []string
	for tabID := range d.tabClosures {
		pendingTabs = append(pendingTabs, tabID)
	}
	d.mu.Unlock()
	sort.Strings(pendingTabs)
	for _, tabID := range pendingTabs {
		if err := d.cleanupOwnedTab(tabID); err != nil {
			return fmt.Errorf("recover tab closure: %w", err)
		}
	}

	if err := d.recoverTabCreateIntents(); err != nil {
		return err
	}

	d.mu.Lock()
	var stale []string
	for tabID := range d.ownedTabs {
		if !d.hasActivePlacementLocked(tabID) {
			stale = append(stale, tabID)
		}
	}
	d.mu.Unlock()
	sort.Strings(stale)
	for _, tabID := range stale {
		if err := d.cleanupOwnedTab(tabID); err != nil {
			return fmt.Errorf("recover placement tabs: %w", err)
		}
	}
	return nil
}

// recoverTabCreateIntents converges a crash around tab.create without ever
// treating the requested user label as proof of ownership. A unique temporary
// label is the only available attribution because Herdr protocol 16 has no
// atomic idempotency key. One match is claimed durably and rolled back by id;
// zero means create never landed (or was already rolled back). Multiple
// matches are fundamentally unattributable, so the deterministic safe policy
// is to close none and resolve the intent rather than risk deleting an
// external tab.
func (d *Daemon) recoverTabCreateIntents() error {
	d.mu.Lock()
	var ids []string
	for id := range d.tabCreateIntents {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	sort.Strings(ids)

	for _, id := range ids {
		if err := d.recoverTabCreateIntent(id); err != nil {
			return err
		}
	}
	return nil
}

func (d *Daemon) recoverTabCreateIntent(id string) error {
	d.mu.Lock()
	intent, ok := d.tabCreateIntents[id]
	d.mu.Unlock()
	if !ok {
		return nil
	}
	tabs, err := herdrwire.TabList(d.session.SocketPath, intent.WorkspaceID)
	if err != nil {
		workspaces, listErr := herdrwire.WorkspaceList(d.session.SocketPath)
		if listErr != nil {
			return fmt.Errorf("recover tab creation intent %s: list tabs: %w (verify workspaces: %v)", id, err, listErr)
		}
		workspaceExists := false
		for _, workspace := range workspaces {
			if workspace.WorkspaceID == intent.WorkspaceID {
				workspaceExists = true
				break
			}
		}
		if workspaceExists {
			return fmt.Errorf("recover tab creation intent %s: list tabs: %w", id, err)
		}
		if err := d.resolveTabCreateIntent(id); err != nil {
			return fmt.Errorf("recover tab creation intent %s: %w", id, err)
		}
		return nil
	}

	var matches []herdrwire.Tab
	for _, tab := range tabs {
		if tab.Label == intent.CreateLabel {
			matches = append(matches, tab)
		}
	}
	switch len(matches) {
	case 0:
		if err := d.resolveTabCreateIntent(id); err != nil {
			return fmt.Errorf("recover tab creation intent %s: %w", id, err)
		}
	case 1:
		tab := matches[0]
		d.mu.Lock()
		current, pending := d.tabCreateIntents[id]
		if !pending {
			d.mu.Unlock()
			return nil
		}
		err := d.append(event{
			Event:       evTabCreated,
			IntentID:    id,
			WorkspaceID: current.WorkspaceID,
			TabID:       tab.TabID,
			TabLabel:    current.TabLabel,
		})
		if err == nil {
			delete(d.tabCreateIntents, id)
			d.ownedTabs[tab.TabID] = ownedTab{
				WorkspaceID: current.WorkspaceID,
				TabID:       tab.TabID,
				Label:       current.TabLabel,
			}
		}
		d.mu.Unlock()
		if err != nil {
			return fmt.Errorf("recover tab creation intent %s: journal ownership: %w", id, err)
		}
		if err := d.cleanupOwnedTab(tab.TabID); err != nil {
			return fmt.Errorf("recover tab creation intent %s: rollback tab %s: %w", id, tab.TabID, err)
		}
	default:
		d.debug.Printf("tab creation intent %s has %d temporary-label matches; preserving all as unattributable", id, len(matches))
		if err := d.resolveTabCreateIntent(id); err != nil {
			return fmt.Errorf("recover ambiguous tab creation intent %s: %w", id, err)
		}
	}
	return nil
}

// stopWorkspaceOwner closes a dedicated workspace once, then records every
// nested explicitly placed agent as stopped because its pane disappeared with
// that workspace. Herdr is called only after releasing d.mu.
func (d *Daemon) stopWorkspaceOwner(ownerName string, owner protocol.Agent, reason string) error {
	d.mu.Lock()
	if closure, ok := d.workspaceClosures[owner.WorkspaceID]; ok {
		d.mu.Unlock()
		return d.finishWorkspaceClosure(closure)
	}
	if d.closingWorkspaces[owner.WorkspaceID] {
		d.mu.Unlock()
		return fmt.Errorf("workspace %s is already closing", owner.WorkspaceID)
	}
	d.closingWorkspaces[owner.WorkspaceID] = true
	var launches []*launchLatch
	for name, agent := range d.agents {
		if name == ownerName || agent.WorkspaceID != owner.WorkspaceID || agent.OwnsWorkspace {
			continue
		}
		if launch := d.launches[name]; launch != nil {
			launches = append(launches, launch)
		}
	}
	d.mu.Unlock()

	for _, launch := range launches {
		<-launch.done
	}

	d.mu.Lock()
	var stops []stopRecord
	if current, ok := d.agents[ownerName]; ok && current.State != stateStopped {
		stops = append(stops, stopRecord{Name: ownerName, Reason: reason})
	}
	var nested []string
	for name, agent := range d.agents {
		if name == ownerName || agent.WorkspaceID != owner.WorkspaceID || agent.OwnsWorkspace {
			continue
		}
		if agent.State != stateStopped && agent.State != stateOrphaned {
			nested = append(nested, name)
		}
	}
	sort.Strings(nested)
	for _, name := range nested {
		stops = append(stops, stopRecord{Name: name, Reason: "workspace owner stopped"})
	}
	var ownedTabIDs []string
	for tabID, owned := range d.ownedTabs {
		if owned.WorkspaceID == owner.WorkspaceID {
			ownedTabIDs = append(ownedTabIDs, tabID)
		}
	}
	sort.Strings(ownedTabIDs)
	var tabs []tabRecord
	for _, tabID := range ownedTabIDs {
		owned := d.ownedTabs[tabID]
		tabs = append(tabs, tabRecord{
			WorkspaceID: owned.WorkspaceID,
			TabID:       tabID,
			TabLabel:    owned.Label,
		})
	}
	closure := event{
		Event:       evWorkspaceClosing,
		Name:        ownerName,
		WorkspaceID: owner.WorkspaceID,
		Stops:       stops,
		Tabs:        tabs,
	}
	if err := d.append(closure); err != nil {
		delete(d.closingWorkspaces, owner.WorkspaceID)
		d.mu.Unlock()
		return err
	}
	d.workspaceClosures[owner.WorkspaceID] = closure
	var flights []*inboxNotifyFlight
	for _, stop := range stops {
		d.inboxNotifyArmed[stop.Name] = false
		delete(d.inboxNotifyTasks, stop.Name)
		d.cancelAgentWaitersLocked(stop.Name)
		if flight := d.inboxNotifyFlights[stop.Name]; flight != nil {
			flight.cancel()
			flights = append(flights, flight)
		}
	}
	d.mu.Unlock()
	for _, flight := range flights {
		<-flight.done
	}
	return d.finishWorkspaceClosure(closure)
}

func (d *Daemon) finishWorkspaceClosure(closure event) error {
	workspaceID := closure.WorkspaceID
	d.mu.Lock()
	if run := d.workspaceCloseRuns[workspaceID]; run != nil {
		d.mu.Unlock()
		<-run.done
		return run.err
	}
	run := &closeLatch{done: make(chan struct{})}
	d.workspaceCloseRuns[workspaceID] = run
	d.closingWorkspaces[workspaceID] = true
	d.mu.Unlock()

	err := d.closeWorkspaceIdempotent(workspaceID)

	d.mu.Lock()
	if err == nil {
		journalEvents := make([]event, 0, len(closure.Stops)+len(closure.Tabs)+1)
		for _, stop := range closure.Stops {
			journalEvents = append(journalEvents, event{Event: evStopped, Name: stop.Name, Reason: stop.Reason})
		}
		for _, tab := range closure.Tabs {
			journalEvents = append(journalEvents, event{
				Event:       evTabClosed,
				WorkspaceID: tab.WorkspaceID,
				TabID:       tab.TabID,
				TabLabel:    tab.TabLabel,
			})
		}
		journalEvents = append(journalEvents, event{Event: evWorkspaceClosed, WorkspaceID: workspaceID})
		err = d.appendAll(journalEvents...)
	}
	if err == nil {
		for _, stop := range closure.Stops {
			d.setStoppedLocked(stop.Name)
		}
		for _, tab := range closure.Tabs {
			delete(d.ownedTabs, tab.TabID)
			delete(d.tabClosures, tab.TabID)
			delete(d.closingTabs, tab.TabID)
		}
		delete(d.workspaceClosures, workspaceID)
		delete(d.closingWorkspaces, workspaceID)
	}
	run.err = err
	delete(d.workspaceCloseRuns, workspaceID)
	close(run.done)
	d.mu.Unlock()
	return err
}

func (d *Daemon) closeWorkspaceIdempotent(workspaceID string) error {
	err := herdrwire.WorkspaceClose(d.session.SocketPath, workspaceID)
	if err == nil {
		return nil
	}
	workspaces, listErr := herdrwire.WorkspaceList(d.session.SocketPath)
	if listErr != nil {
		return fmt.Errorf("close workspace %s: %w (verify workspaces: %v)", workspaceID, err, listErr)
	}
	for _, workspace := range workspaces {
		if workspace.WorkspaceID == workspaceID {
			return fmt.Errorf("close workspace %s: %w", workspaceID, err)
		}
	}
	return nil
}
