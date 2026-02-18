package workflow

import (
	"context"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// ExecutionOption
// ─────────────────────────────────────────────────────────────────────────────

// ExecutionOption configures an Execution at construction time.
type ExecutionOption[T any] func(*Execution[T])

// WithHooks attaches lifecycle callbacks to the Execution.
//
//	exec := wf.NewExecution(ctx, "PENDING", workflow.WithHooks(workflow.ExecutionHooks[*Order]{
//	    OnTransition: func(ctx context.Context, from, to, signal string, o *Order) {
//	        log.Printf("order %s: %s --%s--> %s", o.ID, from, signal, to)
//	    },
//	    OnError: func(ctx context.Context, state, signal string, err error, o *Order) {
//	        log.Printf("order %s: signal %s in state %s failed: %v", o.ID, signal, state, err)
//	    },
//	}))
func WithHooks[T any](hooks ExecutionHooks[T]) ExecutionOption[T] {
	return func(e *Execution[T]) { e.hooks = hooks }
}

// ─────────────────────────────────────────────────────────────────────────────
// Execution
// ─────────────────────────────────────────────────────────────────────────────

// Execution is a thread-safe, stateful wrapper around a Workflow — the
// go-statemachine equivalent of a Temporal Workflow Execution.
// It tracks the current state, event history, and lifecycle hooks for one entity.
//
// Create via Workflow.NewExecution; share across goroutines freely.
type Execution[T any] struct {
	// signalMu serialises Signal calls so transitions are never concurrent.
	// Activities run while signalMu is held but mu is NOT held, so state
	// reads (CurrentState, Await, etc.) remain responsive during long activities.
	signalMu sync.Mutex

	mu        sync.Mutex
	cond      *sync.Cond // guards state changes; used by Await
	workflow  *Workflow[T]
	state     string
	history   []HistoryEntry
	hooks     ExecutionHooks[T]
	cancelCtx context.Context
	cancelFn  context.CancelFunc
}

// Signal drives the Execution to its next state via signal.
//
// Activities receive the Execution's lifecycle context, so calling Cancel()
// will interrupt in-flight Activities that respect ctx.Done().
//
// Signal is safe for concurrent callers; signals are serialised internally.
func (e *Execution[T]) Signal(ctx context.Context, signal string, payload T) error {
	// Serialise signal processing (activities run under this lock, not mu).
	e.signalMu.Lock()
	defer e.signalMu.Unlock()

	// Snapshot current state (brief mu hold).
	e.mu.Lock()
	if e.cancelCtx.Err() != nil {
		e.mu.Unlock()
		return ErrExecutionCancelled
	}
	from := e.state
	e.mu.Unlock()

	// Run the transition (activities run here, outside mu).
	start := time.Now()
	newState, err := e.workflow.Signal(e.cancelCtx, from, signal, payload)
	duration := time.Since(start)

	// Record history and advance state (brief mu hold).
	entry := HistoryEntry{
		Signal:    signal,
		FromState: from,
		ToState:   newState,
		At:        start,
		Duration:  duration,
		Err:       err,
	}
	e.mu.Lock()
	e.state = newState
	e.history = append(e.history, entry)
	e.mu.Unlock()

	// Wake any Await waiters.
	e.cond.Broadcast()

	// Fire lifecycle hooks (outside all locks so hooks can call Signal).
	if err != nil {
		if e.hooks.OnError != nil {
			e.hooks.OnError(ctx, from, signal, err, payload)
		}
		return err
	}
	if e.hooks.OnTransition != nil {
		e.hooks.OnTransition(ctx, from, newState, signal, payload)
	}
	return nil
}

// CurrentState returns the current state of the Execution.
func (e *Execution[T]) CurrentState() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// CanReceive reports whether signal is accepted in the current state.
func (e *Execution[T]) CanReceive(signal string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.workflow.routes[e.state][signal]
	return ok
}

// SetState forcibly overrides the current state without executing any
// Activities or hooks. Useful for initialising from persisted state or testing.
func (e *Execution[T]) SetState(state string) {
	e.mu.Lock()
	e.state = state
	e.mu.Unlock()
	e.cond.Broadcast()
}

// AvailableSignals returns the signals accepted in the current state.
func (e *Execution[T]) AvailableSignals() []string {
	e.mu.Lock()
	state := e.state
	e.mu.Unlock()
	return e.workflow.AvailableSignals(state)
}

// History returns a snapshot copy of all recorded signal attempts.
func (e *Execution[T]) History() []HistoryEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]HistoryEntry, len(e.history))
	copy(out, e.history)
	return out
}

// Cancel cancels the Execution's lifecycle context. Any in-flight Activity that
// respects ctx.Done() will be interrupted. Subsequent calls to Signal return
// ErrExecutionCancelled immediately.
func (e *Execution[T]) Cancel() {
	e.cancelFn()
	e.cond.Broadcast()
}

// Done returns a channel that is closed when the Execution is cancelled.
// Equivalent to the context passed to NewExecution after calling Cancel.
func (e *Execution[T]) Done() <-chan struct{} {
	return e.cancelCtx.Done()
}

// Await blocks until condition(currentState) returns true, the Execution is
// cancelled, or ctx is done — whichever occurs first.
//
// The current state is passed directly to condition so that the predicate does
// not need to call CurrentState() (which would deadlock by re-acquiring the
// internal lock):
//
//	err := exec.Await(ctx, func(state string) bool {
//	    return state == "APPROVED"
//	})
func (e *Execution[T]) Await(ctx context.Context, condition func(state string) bool) error {
	// Broadcast when the caller's context is cancelled so cond.Wait unblocks.
	stopCh := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			e.cond.Broadcast()
		case <-stopCh:
		}
	}()
	defer close(stopCh)

	e.mu.Lock()
	defer e.mu.Unlock()

	for !condition(e.state) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e.cancelCtx.Err() != nil {
			return ErrExecutionCancelled
		}
		// cond.Wait atomically releases mu and suspends; reacquires on wake.
		e.cond.Wait()
	}
	return nil
}
