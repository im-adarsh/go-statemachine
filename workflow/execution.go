package workflow

import (
	"context"
	"sync"
)

// Execution is a stateful wrapper around a Workflow — analogous to a running
// Temporal Workflow Execution. It tracks the current state and is safe for
// concurrent use.
//
// Create one per entity (order, user, document, …) with Workflow.NewExecution.
// To drive the same entity from multiple goroutines, share a single Execution;
// its internal mutex serialises all state transitions.
type Execution struct {
	mu       sync.Mutex
	workflow *Workflow
	state    string
}

// Signal receives the named signal and drives the Execution to the next state.
// It delegates to Workflow.Signal and updates the internal state on success.
//
// Returns an error on unknown signals, failed Activities, or failed hooks.
// The state is unchanged on error (except for OnEnter failures — see
// Workflow.Signal for the precise semantics).
func (e *Execution) Signal(ctx context.Context, signal string, payload any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	newState, err := e.workflow.Signal(ctx, e.state, signal, payload)
	e.state = newState // always adopt the returned state (may be unchanged on error)
	return err
}

// CurrentState returns the current state — analogous to a Temporal Query.
// Safe to call concurrently with Signal.
func (e *Execution) CurrentState() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// CanReceive reports whether the given signal can be received from the current
// state. Useful for driving UI (disabling buttons, hiding actions).
func (e *Execution) CanReceive(signal string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.workflow.routes[e.state][signal]
	return ok
}

// SetState overrides the current state without triggering any transitions or
// Activities. Intended for testing, manual correction, or event-sourced replay.
func (e *Execution) SetState(state string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = state
}
