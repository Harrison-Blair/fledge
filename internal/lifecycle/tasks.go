package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
)

func (m *Manager) coordinationStore(dir string) (caller, root string, store *messaging.Store, err error) {
	root, err = project.Find(dir)
	if err != nil {
		return "", "", nil, err
	}
	record, found, err := readRecord(root)
	if err != nil {
		return "", "", nil, err
	}
	if !found {
		return "", "", nil, errors.New("project has no Fledge session; run fledge start first")
	}
	store = messaging.New(root, record.SessionName)
	if err := validateStoreBinding(store, record); err != nil {
		return "", "", nil, err
	}
	callerValue, err := m.paneCaller(store)
	if err != nil {
		return "", "", nil, err
	}
	return callerValue.identity, root, store, nil
}

func validateStoreBinding(store *messaging.Store, record record) error {
	if record.MessagingSessionID == "" {
		return errors.New("Fledge session record is not bound to durable session state; run fledge start to validate and upgrade it")
	}
	sessionID, err := store.SessionID()
	if err != nil {
		return err
	}
	if sessionID != record.MessagingSessionID {
		return fmt.Errorf("%w: record has %q, log has %q", messaging.ErrSessionMismatch, record.MessagingSessionID, sessionID)
	}
	return nil
}

// AgentList reads the durable registry without querying Herdr.
func (m *Manager) AgentList(_ context.Context, dir string) ([]messaging.Agent, error) {
	_, _, store, err := m.coordinationStore(dir)
	if err != nil {
		return nil, err
	}
	return store.Agents()
}

func (m *Manager) TaskAssign(_ context.Context, dir, assignee, parent, description string) (messaging.Task, error) {
	caller, root, store, err := m.coordinationStore(dir)
	if err != nil {
		return messaging.Task{}, err
	}
	task, err := store.AssignTask(caller, assignee, parent, description)
	return task, m.deliverThrough(root, err)
}

func (m *Manager) TaskProgress(_ context.Context, dir, id, detail string) (messaging.Task, error) {
	caller, _, store, err := m.coordinationStore(dir)
	if err != nil {
		return messaging.Task{}, err
	}
	return store.RecordProgress(caller, id, detail)
}

func (m *Manager) TaskTransition(_ context.Context, dir, id string, status messaging.TaskStatus, detail string) (messaging.Task, error) {
	caller, root, store, err := m.coordinationStore(dir)
	if err != nil {
		return messaging.Task{}, err
	}
	task, err := store.TransitionTask(caller, id, status, detail)
	return task, m.deliverThrough(root, err)
}

// deliverThrough makes sure a dispatcher exists to deliver the wakes a
// successful command just appended. Launching is idempotent — a second daemon
// exits as soon as it finds the singleton held — and a dispatcher that cannot
// be started only warns: the events are already durable, and the next command
// or spawn tries again.
func (m *Manager) deliverThrough(root string, err error) error {
	if err != nil {
		return err
	}
	m.launchWatcherWarn(root)
	return nil
}

func (m *Manager) TaskList(_ context.Context, dir string) ([]messaging.Task, error) {
	caller, _, store, err := m.coordinationStore(dir)
	if err != nil {
		return nil, err
	}
	tasks, err := store.Tasks()
	if err != nil || caller == userIdentity || caller == orchestratorIdentity {
		return tasks, err
	}
	visible := make(map[string]bool)
	for _, task := range tasks {
		if task.Assignee == caller || task.Assigner == caller {
			visible[task.ID] = true
		}
	}
	changed := true
	for changed {
		changed = false
		for _, task := range tasks {
			if visible[task.ID] && task.ParentID != "" && !visible[task.ParentID] {
				visible[task.ParentID], changed = true, true
			}
		}
	}
	filtered := tasks[:0]
	for _, task := range tasks {
		if visible[task.ID] {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (m *Manager) TaskShow(ctx context.Context, dir, id string) (messaging.Task, error) {
	tasks, err := m.TaskList(ctx, dir)
	if err != nil {
		return messaging.Task{}, err
	}
	for _, task := range tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return messaging.Task{}, fmt.Errorf("%w: %s", messaging.ErrTaskNotFound, id)
}
