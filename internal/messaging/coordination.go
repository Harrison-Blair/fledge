package messaging

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TaskStatus is the durable state of one unit of delegated work.
type TaskStatus string

const (
	TaskActive        TaskStatus = "active"
	TaskBlocked       TaskStatus = "blocked"
	TaskNeedsDecision TaskStatus = "needs-decision"
	TaskCompleted     TaskStatus = "completed"
	TaskFailed        TaskStatus = "failed"
	TaskCanceled      TaskStatus = "canceled"
	TaskOrphaned      TaskStatus = "orphaned"
)

// UserIdentity and OrchestratorIdentity are the two privileged coordination
// identities. Both bypass delegation and transition authorization, so neither
// name may be claimed by a spawned agent.
const (
	UserIdentity         = "user"
	OrchestratorIdentity = "orchestrator"
)

var (
	ErrAgentNotFound = errors.New("agent not found")
	ErrTaskNotFound  = errors.New("task not found")
	ErrCapacity      = errors.New("agent has no task capacity")
)

// Agent is the registry projection for a named, pane-bound agent process.
type Agent struct {
	Name          string
	PaneID        string
	Harness       string
	AuthorityHash string
	CanDelegate   bool
	ParentTaskID  string
	Active        bool
	Status        string
	RegisteredAt  time.Time
	UpdatedAt     time.Time
}

// Task is the reconstructed view of one assignment.
type Task struct {
	ID          string
	ParentID    string
	Assignee    string
	Assigner    string
	Description string
	Status      TaskStatus
	Detail      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Wake is a stable dispatcher delivery. A requested wake is replayed until a
// terminal delivery outcome is durably recorded.
type Wake struct {
	ID            string
	Kind          string
	ReferenceID   string
	Recipient     string
	RecipientPane string
	Body          string
	Status        Status
	Failure       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RegisterParams describes one pane that has successfully started. Task is
// optional; when present the registration, assignment, and wake request are
// appended as one fsynced transaction.
type RegisterParams struct {
	Name, PaneID, Harness, AuthorityHash, ParentTaskID, Caller, Task string
	CanDelegate                                                      bool
}

func validateCoordinationEvent(e event) error {
	switch e.Type {
	case eventAgentRegistered:
		if blank(e.AgentName, e.PaneID, e.Harness) {
			return errors.New("invalid agent_registered fields")
		}
		if e.AuthorityHash != "" && !validAuthorityHash(e.AuthorityHash) {
			return errors.New("agent_registered has an invalid authority hash")
		}
	case eventAgentStopped:
		if blank(e.AgentName, e.PaneID) {
			return errors.New("invalid agent_stopped fields")
		}
	case eventAgentStatus:
		if blank(e.AgentName, e.PaneID, e.Detail) {
			return errors.New("invalid agent_status_changed fields")
		}
	case eventTaskAssigned:
		if blank(e.TaskID, e.Assignee, e.Assigner, e.Description) || e.TaskStatus != TaskActive {
			return errors.New("invalid task_assigned fields")
		}
	case eventTaskProgress, eventTaskBlocked, eventTaskDecision, eventTaskResumed,
		eventTaskCompleted, eventTaskFailed, eventTaskCanceled, eventTaskOrphaned:
		if blank(e.TaskID, string(e.TaskStatus)) {
			return errors.New("task transition is missing fields")
		}
	case eventWakeRequested:
		if blank(e.WakeID, e.WakeKind, e.Recipient, e.RecipientPane, e.Body) {
			return errors.New("invalid wake_requested fields")
		}
	case eventWakeAttempt:
		if blank(e.WakeID) {
			return errors.New("invalid wake_attempt fields")
		}
	case eventWakeOutcome:
		if blank(e.WakeID) || e.Accepted == nil {
			return errors.New("invalid wake_outcome fields")
		}
	}
	return nil
}

func validAuthorityHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func blank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func applyCoordinationEvent(state *logState, e event) error {
	switch e.Type {
	case eventAgentRegistered:
		if current, ok := state.agents[e.AgentName]; ok && current.Active {
			return fmt.Errorf("agent %q is already registered", e.AgentName)
		}
		for _, current := range state.agents {
			if current.Active && current.PaneID == e.PaneID {
				return fmt.Errorf("pane %q is already registered to %q", e.PaneID, current.Name)
			}
		}
		state.agents[e.AgentName] = Agent{Name: e.AgentName, PaneID: e.PaneID, Harness: e.Harness,
			AuthorityHash: e.AuthorityHash, CanDelegate: e.CanDelegate, ParentTaskID: e.ParentTaskID, Active: true,
			Status: "working", RegisteredAt: e.At, UpdatedAt: e.At}
	case eventAgentStopped:
		agent, ok := state.agents[e.AgentName]
		if !ok || !agent.Active || agent.PaneID != e.PaneID {
			return fmt.Errorf("agent_stopped references unknown active pane")
		}
		agent.Active, agent.Status, agent.UpdatedAt = false, "stopped", e.At
		state.agents[e.AgentName] = agent
	case eventAgentStatus:
		agent, ok := state.agents[e.AgentName]
		if !ok || !agent.Active || agent.PaneID != e.PaneID {
			return fmt.Errorf("agent status references unknown active pane")
		}
		agent.Status, agent.UpdatedAt = e.Detail, e.At
		state.agents[e.AgentName] = agent
	case eventTaskAssigned:
		if _, exists := state.tasks[e.TaskID]; exists {
			return fmt.Errorf("duplicate task ID %q", e.TaskID)
		}
		if e.ParentTaskID != "" {
			if _, exists := state.tasks[e.ParentTaskID]; !exists {
				return fmt.Errorf("parent task %q does not exist", e.ParentTaskID)
			}
		}
		state.tasks[e.TaskID] = Task{ID: e.TaskID, ParentID: e.ParentTaskID, Assignee: e.Assignee,
			Assigner: e.Assigner, Description: e.Description, Status: TaskActive,
			CreatedAt: e.At, UpdatedAt: e.At}
		state.taskOrder = append(state.taskOrder, e.TaskID)
	case eventTaskProgress, eventTaskBlocked, eventTaskDecision, eventTaskResumed,
		eventTaskCompleted, eventTaskFailed, eventTaskCanceled, eventTaskOrphaned:
		task, ok := state.tasks[e.TaskID]
		if !ok {
			return fmt.Errorf("task transition references unknown task %q", e.TaskID)
		}
		task.Status, task.Detail, task.UpdatedAt = e.TaskStatus, e.Detail, e.At
		state.tasks[e.TaskID] = task
	case eventWakeRequested:
		if _, exists := state.wakes[e.WakeID]; exists {
			return fmt.Errorf("duplicate wake ID %q", e.WakeID)
		}
		state.wakes[e.WakeID] = Wake{ID: e.WakeID, Kind: e.WakeKind, ReferenceID: e.TaskID,
			Recipient: e.Recipient, RecipientPane: e.RecipientPane, Body: e.Body,
			Status: StatusPending, CreatedAt: e.At, UpdatedAt: e.At}
		state.wakeOrder = append(state.wakeOrder, e.WakeID)
	case eventWakeAttempt:
		wake, ok := state.wakes[e.WakeID]
		if !ok || wake.Status != StatusPending {
			return fmt.Errorf("wake attempt references non-pending wake %q", e.WakeID)
		}
		wake.Status, wake.UpdatedAt = StatusUncertain, e.At
		state.wakes[e.WakeID] = wake
		projectMessageStatus(state, wake)
	case eventWakeOutcome:
		wake, ok := state.wakes[e.WakeID]
		if !ok || wake.Status != StatusUncertain {
			return fmt.Errorf("wake outcome references non-attempted wake %q", e.WakeID)
		}
		wake.UpdatedAt = e.At
		if *e.Accepted {
			wake.Status = StatusDelivered
		} else {
			wake.Status, wake.Failure = StatusFailed, e.Detail
		}
		state.wakes[e.WakeID] = wake
		projectMessageStatus(state, wake)
	}
	return nil
}

// projectMessageStatus mirrors a message wake's delivery status onto the message
// it carries. Task and agent wakes reference no message and are ignored.
func projectMessageStatus(state *logState, wake Wake) {
	if wake.Kind != "message" {
		return
	}
	message, ok := state.messages[wake.ReferenceID]
	if !ok {
		return
	}
	message.Status = wake.Status
	state.messages[wake.ReferenceID] = message
}

// RegisterAgent atomically records a pane and its optional initial task.
func (s *Store) RegisterAgent(params RegisterParams) (Agent, *Task, error) {
	var result Agent
	var initial *Task
	err := s.withState(func(state *logState) error {
		if blank(params.Name, params.PaneID, params.Harness, params.Caller) {
			return errors.New("agent registration is missing fields")
		}
		if _, exists := activeAgent(state, params.Name); exists {
			return fmt.Errorf("agent %q is already registered", params.Name)
		}
		if err := authorizeDelegation(state, params.Caller, params.ParentTaskID); err != nil {
			return err
		}
		if err := validateParentTask(state, params.ParentTaskID); err != nil {
			return err
		}
		at := s.now()
		events := []event{{Version: eventVersion, Type: eventAgentRegistered, At: at, SessionID: state.sessionID,
			AgentName: params.Name, PaneID: params.PaneID, Harness: params.Harness,
			AuthorityHash: params.AuthorityHash, CanDelegate: params.CanDelegate, ParentTaskID: params.ParentTaskID}}
		if strings.TrimSpace(params.Task) != "" {
			taskID, err := s.uniqueID(state, "t-", taskOrWakeTaken(state))
			if err != nil {
				return err
			}
			events = append(events, event{Version: eventVersion, Type: eventTaskAssigned, At: at, SessionID: state.sessionID,
				TaskID: taskID, ParentTaskID: params.ParentTaskID, Assignee: params.Name,
				Assigner: params.Caller, Description: strings.TrimSpace(params.Task), TaskStatus: TaskActive})
			wake, err := s.wakeFor(state, at, "task-assigned", taskID, params.Name, params.PaneID,
				fmt.Sprintf("[Fledge task]\nID: %s\nAssigned by: %s\nTask:\n%s\n\nReport progress with: fledge agent task progress %s <text> — finish with fledge agent task complete %s --file <path> (its summary reaches me; no separate message needed).", taskID, params.Caller, strings.TrimSpace(params.Task), taskID, taskID), false)
			if err != nil {
				return err
			}
			events = append(events, *wake)
		}
		if err := s.commit(state, events); err != nil {
			return err
		}
		result = state.agents[params.Name]
		if len(events) > 1 {
			task := state.tasks[events[1].TaskID]
			initial = &task
		}
		return nil
	})
	return result, initial, err
}

func authorizeDelegation(state *logState, caller, parentID string) error {
	if caller == UserIdentity || caller == OrchestratorIdentity {
		return nil
	}
	agent, ok := activeAgent(state, caller)
	if !ok || !agent.CanDelegate {
		return fmt.Errorf("%w: agent %q cannot delegate", ErrUnauthorized, caller)
	}
	if parentID == "" {
		return fmt.Errorf("%w: delegated work requires --parent-task", ErrUnauthorized)
	}
	parent, ok := state.tasks[parentID]
	if !ok || parent.Assignee != caller || terminal(parent.Status) {
		return fmt.Errorf("%w: parent task %q is not active work owned by %q", ErrUnauthorized, parentID, caller)
	}
	return nil
}

func validateParentTask(state *logState, parentID string) error {
	if parentID == "" {
		return nil
	}
	parent, ok := state.tasks[parentID]
	if !ok {
		return fmt.Errorf("%w: parent %s", ErrTaskNotFound, parentID)
	}
	if terminal(parent.Status) {
		return fmt.Errorf("parent task %s is terminal", parentID)
	}
	return nil
}

// StopAgent records pane departure and orphans every unfinished assignment.
func (s *Store) StopAgent(name, paneID string) error {
	return s.withState(func(state *logState) error {
		agent, ok := activeAgent(state, name)
		if !ok || agent.PaneID != paneID {
			return fmt.Errorf("%w: %s", ErrAgentNotFound, name)
		}
		at := s.now()
		events := []event{{Version: eventVersion, Type: eventAgentStopped, At: at, SessionID: state.sessionID, AgentName: name, PaneID: paneID}}
		for _, id := range state.taskOrder {
			task := state.tasks[id]
			if task.Assignee == name && !terminal(task.Status) {
				events = append(events, event{Version: eventVersion, Type: eventTaskOrphaned, At: at, SessionID: state.sessionID,
					TaskID: id, TaskStatus: TaskOrphaned, Detail: "assignee pane stopped"})
				if wake, err := s.wakeFor(state, at, "task-orphaned", id, task.Assigner, "",
					fmt.Sprintf("Task %s became orphaned because agent %s stopped.", id, name), true); err != nil {
					return err
				} else if wake != nil {
					events = append(events, *wake)
				}
			}
		}
		return s.commit(state, events)
	})
}

// StopAgentByPane consumes a Herdr pane.closed event. Replayed or late events
// are harmless because an inactive/missing pane has already been projected.
func (s *Store) StopAgentByPane(paneID string) error {
	agent, err := s.AgentByPane(paneID)
	if errors.Is(err, ErrAgentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.StopAgent(agent.Name, paneID)
}

func (s *Store) Agents() ([]Agent, error) {
	var result []Agent
	err := s.withState(func(state *logState) error {
		for _, agent := range state.agents {
			result = append(result, agent)
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		return nil
	})
	return result, err
}

func (s *Store) AgentByPane(paneID string) (Agent, error) {
	var result Agent
	err := s.withState(func(state *logState) error {
		for _, agent := range state.agents {
			if agent.Active && agent.PaneID == paneID {
				result = agent
				return nil
			}
		}
		return fmt.Errorf("%w for pane %q", ErrAgentNotFound, paneID)
	})
	return result, err
}

// AgentByPaneAny resolves the agent bound to paneID whether or not it is still
// active, preferring the active binding when a pane has been reused. Authority
// checks need the inactive record: a stopped agent's pane must be recognized as
// a stopped agent rather than mistaken for an unmanaged direct-user shell.
func (s *Store) AgentByPaneAny(paneID string) (Agent, error) {
	var result Agent
	err := s.withState(func(state *logState) error {
		found := false
		for _, agent := range state.agents {
			if agent.PaneID != paneID {
				continue
			}
			if agent.Active {
				result = agent
				return nil
			}
			if !found || agent.UpdatedAt.After(result.UpdatedAt) {
				result, found = agent, true
			}
		}
		if !found {
			return fmt.Errorf("%w for pane %q", ErrAgentNotFound, paneID)
		}
		return nil
	})
	return result, err
}

// AgentByAuthorityHashAny resolves the most recent pane bound to a local
// authority secret. Only hashes are durable, so reading the audit does not
// reveal the bearer token needed to speak for another pane.
func (s *Store) AgentByAuthorityHashAny(authorityHash string) (Agent, error) {
	var result Agent
	err := s.withState(func(state *logState) error {
		found := false
		for _, agent := range state.agents {
			if agent.AuthorityHash == "" || agent.AuthorityHash != authorityHash {
				continue
			}
			if agent.Active {
				result = agent
				return nil
			}
			if !found || agent.UpdatedAt.After(result.UpdatedAt) {
				result, found = agent, true
			}
		}
		if !found {
			return fmt.Errorf("%w for authority binding", ErrAgentNotFound)
		}
		return nil
	})
	return result, err
}

func (s *Store) Agent(name string) (Agent, error) {
	var result Agent
	err := s.withState(func(state *logState) error {
		var ok bool
		result, ok = activeAgent(state, name)
		if !ok {
			return fmt.Errorf("%w: %s", ErrAgentNotFound, name)
		}
		return nil
	})
	return result, err
}

// RecordAgentStatus consumes one pane-bound Herdr push event. The wake matrix
// is deliberately small: blocked and unexpectedly idle/terminal work wake the
// task's assigner; working merely re-arms the registry projection.
func (s *Store) RecordAgentStatus(paneID, status string) error {
	return s.withState(func(state *logState) error {
		var agent Agent
		for _, candidate := range state.agents {
			if candidate.Active && candidate.PaneID == paneID {
				agent = candidate
				break
			}
		}
		if agent.Name == "" {
			return nil
		}
		if agent.Status == status {
			return nil
		}
		at := s.now()
		events := []event{{Version: eventVersion, Type: eventAgentStatus, At: at, SessionID: state.sessionID,
			AgentName: agent.Name, PaneID: paneID, Detail: status}}
		var active *Task
		for _, id := range state.taskOrder {
			task := state.tasks[id]
			if task.Assignee == agent.Name && !terminal(task.Status) {
				copy := task
				active = &copy
				break
			}
		}
		if active != nil && (status == "blocked" || status == "idle" || status == "failed" || status == "stopped") {
			body := fmt.Sprintf("Agent %s entered Herdr status %s while owning task %s.", agent.Name, status, active.ID)
			wake, err := s.wakeFor(state, at, "agent-"+status, active.ID, active.Assigner, "", body, true)
			if err != nil {
				return err
			}
			if wake != nil {
				events = append(events, *wake)
			}
		}
		return s.commit(state, events)
	})
}

func activeAgent(state *logState, name string) (Agent, bool) {
	agent, ok := state.agents[name]
	return agent, ok && agent.Active
}

func ensureCapacity(state *logState, assignee string) error {
	if _, ok := activeAgent(state, assignee); !ok {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, assignee)
	}
	for _, task := range state.tasks {
		if task.Assignee == assignee && !terminal(task.Status) {
			return fmt.Errorf("%w: %s is already assigned task %s", ErrCapacity, assignee, task.ID)
		}
	}
	return nil
}

// AssignTask creates one assignment and its dispatcher wake transactionally.
func (s *Store) AssignTask(caller, assignee, parentID, description string) (Task, error) {
	var result Task
	err := s.withState(func(state *logState) error {
		if strings.TrimSpace(description) == "" {
			return errors.New("task description must not be blank")
		}
		if err := authorizeDelegation(state, caller, parentID); err != nil {
			return err
		}
		if err := validateParentTask(state, parentID); err != nil {
			return err
		}
		if err := ensureCapacity(state, assignee); err != nil {
			return err
		}
		id, err := s.uniqueID(state, "t-", taskOrWakeTaken(state))
		if err != nil {
			return err
		}
		at := s.now()
		assigned := event{Version: eventVersion, Type: eventTaskAssigned, At: at, SessionID: state.sessionID,
			TaskID: id, ParentTaskID: parentID, Assignee: assignee, Assigner: caller,
			Description: strings.TrimSpace(description), TaskStatus: TaskActive}
		wake, err := s.wakeFor(state, at, "task-assigned", id, assignee, "",
			fmt.Sprintf("[Fledge task]\nID: %s\nAssigned by: %s\nTask:\n%s", id, caller, strings.TrimSpace(description)), false)
		if err != nil {
			return err
		}
		if err := s.commit(state, []event{assigned, *wake}); err != nil {
			return err
		}
		result = state.tasks[id]
		return nil
	})
	return result, err
}

func (s *Store) Tasks() ([]Task, error) {
	var result []Task
	err := s.withState(func(state *logState) error {
		for _, id := range state.taskOrder {
			result = append(result, state.tasks[id])
		}
		return nil
	})
	return result, err
}

// TransitionTask validates ownership and the state machine, cascades cancel
// through descendants, and enqueues only the wake required by the transition.
func (s *Store) TransitionTask(caller, id string, target TaskStatus, detail string) (Task, error) {
	var result Task
	err := s.withState(func(state *logState) error {
		task, ok := state.tasks[id]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTaskNotFound, id)
		}
		if err := authorizeTransition(caller, task, target); err != nil {
			return err
		}
		if err := validTransition(task.Status, target); err != nil {
			return err
		}
		if (target == TaskBlocked || target == TaskNeedsDecision || target == TaskFailed) && strings.TrimSpace(detail) == "" {
			return errors.New("transition detail must not be blank")
		}
		if target == TaskCompleted {
			for _, child := range state.tasks {
				if child.ParentID == id && !terminal(child.Status) {
					return fmt.Errorf("task %s has unfinished child %s", id, child.ID)
				}
			}
		}
		at := s.now()
		events := []event{s.taskTransitionEvent(state, at, caller, id, target, strings.TrimSpace(detail))}
		if target == TaskCanceled || target == TaskFailed {
			for _, descendant := range descendants(state, id) {
				if !terminal(descendant.Status) {
					events = append(events, s.taskTransitionEvent(state, at, caller, descendant.ID, TaskCanceled, "ancestor task ended"))
				}
			}
		}
		for _, changed := range append([]event(nil), events...) {
			changedTask := state.tasks[changed.TaskID]
			recipient := changedTask.Assigner
			kind := "task-" + string(changed.TaskStatus)
			if changed.TaskStatus == TaskActive {
				recipient = changedTask.Assignee
				kind = "task-resumed"
			}
			if changed.TaskStatus == TaskCanceled {
				recipient = changedTask.Assignee
			}
			if changed.Type != eventTaskProgress {
				body := fmt.Sprintf("Task %s is now %s", changed.TaskID, changed.TaskStatus)
				if changed.Detail != "" {
					body += ": " + changed.Detail
				}
				wake, err := s.wakeFor(state, at, kind, changed.TaskID, recipient, "", body, true)
				if err != nil {
					return err
				}
				if wake != nil {
					events = append(events, *wake)
				}
			}
		}
		if err := s.commit(state, events); err != nil {
			return err
		}
		result = state.tasks[id]
		return nil
	})
	return result, err
}

func authorizeTransition(caller string, task Task, target TaskStatus) error {
	if caller == UserIdentity || caller == OrchestratorIdentity {
		return nil
	}
	if target == TaskCanceled || target == TaskActive {
		if caller == task.Assigner || caller == task.Assignee {
			return nil
		}
	} else if caller == task.Assignee {
		return nil
	}
	return fmt.Errorf("%w: %q cannot transition task %s", ErrUnauthorized, caller, task.ID)
}

func validTransition(from, to TaskStatus) error {
	valid := false
	switch to {
	case TaskActive:
		valid = from == TaskBlocked || from == TaskNeedsDecision
	case TaskBlocked, TaskNeedsDecision:
		valid = from == TaskActive
	case TaskCompleted, TaskFailed:
		valid = from == TaskActive || from == TaskBlocked || from == TaskNeedsDecision
	case TaskCanceled:
		valid = !terminal(from)
	}
	if !valid {
		return fmt.Errorf("invalid task transition from %s to %s", from, to)
	}
	return nil
}

func terminal(status TaskStatus) bool {
	return status == TaskCompleted || status == TaskFailed || status == TaskCanceled || status == TaskOrphaned
}

func descendants(state *logState, parent string) []Task {
	var result []Task
	for _, id := range state.taskOrder {
		task := state.tasks[id]
		for ancestor := task.ParentID; ancestor != ""; {
			if ancestor == parent {
				result = append(result, task)
				break
			}
			value, ok := state.tasks[ancestor]
			if !ok {
				break
			}
			ancestor = value.ParentID
		}
	}
	return result
}

// taskTransitionEvent records caller as the transition's actor. For a cascade,
// caller was authorized against the ancestor, not each descendant.
func (s *Store) taskTransitionEvent(state *logState, at time.Time, caller, id string, status TaskStatus, detail string) event {
	kind := map[TaskStatus]string{TaskActive: eventTaskResumed, TaskBlocked: eventTaskBlocked,
		TaskNeedsDecision: eventTaskDecision, TaskCompleted: eventTaskCompleted,
		TaskFailed: eventTaskFailed, TaskCanceled: eventTaskCanceled, TaskOrphaned: eventTaskOrphaned}[status]
	if status == TaskActive && state.tasks[id].Status == TaskActive {
		kind = eventTaskProgress
	}
	return event{Version: eventVersion, Type: kind, At: at, SessionID: state.sessionID, TaskID: id, TaskStatus: status, Detail: detail, Actor: caller}
}

func (s *Store) RecordProgress(caller, id, detail string) (Task, error) {
	var result Task
	err := s.withState(func(state *logState) error {
		task, ok := state.tasks[id]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTaskNotFound, id)
		}
		if caller != UserIdentity && caller != OrchestratorIdentity && caller != task.Assignee {
			return fmt.Errorf("%w", ErrUnauthorized)
		}
		if task.Status != TaskActive {
			return fmt.Errorf("task %s is not active", id)
		}
		if strings.TrimSpace(detail) == "" {
			return errors.New("progress detail must not be blank")
		}
		e := event{Version: eventVersion, Type: eventTaskProgress, At: s.now(), SessionID: state.sessionID, TaskID: id, TaskStatus: TaskActive, Detail: strings.TrimSpace(detail), Actor: caller}
		if err := s.commit(state, []event{e}); err != nil {
			return err
		}
		result = state.tasks[id]
		return nil
	})
	return result, err
}

// wakeFor builds the wake-request event for one recipient. A non-empty pane
// addresses that pane directly, because a recipient's own agent event may not be
// applied yet (as when registration and its first task commit together); an
// empty pane resolves the recipient's active pane. When optional, a user,
// absent, or inactive recipient yields (nil, nil) so a durable transition is
// never blocked by a departed recipient — a worker must still be able to finish,
// fail, or orphan its work after the agent that assigned it is gone.
func (s *Store) wakeFor(state *logState, at time.Time, kind, reference, recipient, pane, body string, optional bool) (*event, error) {
	if optional && (recipient == UserIdentity || recipient == "") {
		return nil, nil
	}
	if pane == "" {
		agent, ok := activeAgent(state, recipient)
		if !ok {
			if optional {
				return nil, nil
			}
			return nil, fmt.Errorf("cannot wake inactive agent %q", recipient)
		}
		pane = agent.PaneID
	}
	// A stable "w-"+reference wake ID makes protocol delivery idempotent; fall
	// back to a fresh ID only when there is no reference or it is already taken.
	id := "w-" + reference
	if reference == "" || state.wakes[id].ID != "" {
		var err error
		if id, err = s.uniqueID(state, "w-", taskOrWakeTaken(state)); err != nil {
			return nil, err
		}
	}
	return &event{Version: eventVersion, Type: eventWakeRequested, At: at, SessionID: state.sessionID,
		WakeID: id, WakeKind: kind, TaskID: reference, Recipient: recipient,
		RecipientPane: pane, Body: body}, nil
}

func (s *Store) PendingWakes() ([]Wake, error) {
	var result []Wake
	err := s.withState(func(state *logState) error {
		for _, id := range state.wakeOrder {
			wake := state.wakes[id]
			if wake.Status == StatusPending || wake.Status == StatusUncertain {
				result = append(result, wake)
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) RecordWakeAttempt(id string) (Wake, error) {
	return s.transitionWake(id, eventWakeAttempt, false, "")
}
func (s *Store) RecordWakeOutcome(id string, accepted bool, detail string) (Wake, error) {
	return s.transitionWake(id, eventWakeOutcome, accepted, detail)
}

func (s *Store) transitionWake(id, kind string, accepted bool, detail string) (Wake, error) {
	var result Wake
	err := s.withState(func(state *logState) error {
		wake, ok := state.wakes[id]
		if !ok {
			return fmt.Errorf("wake %q not found", id)
		}
		// Replayed uncertain deliveries intentionally do not write another attempt;
		// protocol delivery is idempotent by the stable wake ID in the envelope.
		if kind == eventWakeAttempt && wake.Status == StatusUncertain {
			result = wake
			return nil
		}
		e := event{Version: eventVersion, Type: kind, At: s.now(), SessionID: state.sessionID, WakeID: id, Detail: detail}
		if kind == eventWakeOutcome {
			e.Accepted = boolPointer(accepted)
		}
		if err := s.commit(state, []event{e}); err != nil {
			return err
		}
		result = state.wakes[id]
		return nil
	})
	return result, err
}
