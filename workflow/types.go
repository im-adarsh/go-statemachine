// Package workflow provides a stateless, fluent state machine for Go modelled
// after Temporal's concepts: Workflows, Executions, Signals, and Activities.
//
// # Terminology
//
//   - Workflow    — immutable, compiled state graph (build once, share freely).
//   - Execution   — stateful wrapper around a Workflow; one per entity.
//   - Signal      — the named event that drives an Execution to its next state.
//   - Activity    — a generic unit of work; used for transitions and state hooks.
//   - Condition   — a predicate for if-else routing.
//   - SwitchExpr  — extracts a routing key for switch-case routing.
//   - Middleware  — wraps an Activity to add cross-cutting behaviour.
//
// # Type parameter T
//
// The Workflow, Execution, Activity, Condition, and SwitchExpr are all
// parameterised on T — the type of payload passed through every signal.
// This eliminates unchecked type assertions in user code:
//
//	wf, _ := workflow.Define[*Order]().
//	    From("PENDING").On("approve").To("APPROVED").
//	    Activity(func(ctx context.Context, o *Order) error {
//	        return charge(o.Amount) // no type assertion needed
//	    }).
//	    Build()
package workflow

import (
	"context"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Public functional types
// ─────────────────────────────────────────────────────────────────────────────

// Activity is the fundamental unit of work — the go-statemachine equivalent of a
// Temporal Activity function. Used for both transition activities and state hooks.
// If an Activity returns an error the transition is aborted and the state is unchanged
// (except for OnEnter failures where the new state is already set).
type Activity[T any] func(ctx context.Context, payload T) error

// Condition evaluates a predicate for if-else routing.
// Return true to select the associated destination state.
type Condition[T any] func(ctx context.Context, payload T) bool

// SwitchExpr extracts the routing key from the payload/context for switch-case routing.
// The returned value is compared with Case values using ==.
type SwitchExpr[T any] func(ctx context.Context, payload T) any

// Middleware wraps an Activity to add cross-cutting behaviour such as logging,
// tracing, or metrics — analogous to Temporal's Activity interceptors.
//
//	var otelMiddleware workflow.Middleware[*Order] = func(next workflow.Activity[*Order]) workflow.Activity[*Order] {
//	    return func(ctx context.Context, o *Order) error {
//	        span := tracer.StartSpan("activity")
//	        defer span.End()
//	        return next(ctx, o)
//	    }
//	}
type Middleware[T any] func(Activity[T]) Activity[T]

// ─────────────────────────────────────────────────────────────────────────────
// Execution lifecycle types
// ─────────────────────────────────────────────────────────────────────────────

// HistoryEntry records one signal attempt — successful or failed.
type HistoryEntry struct {
	Signal    string
	FromState string
	ToState   string // equals FromState on error (state unchanged)
	At        time.Time
	Duration  time.Duration
	Err       error // nil on success
}

// ExecutionHooks provides optional lifecycle callbacks for an Execution.
type ExecutionHooks[T any] struct {
	// OnTransition is called after every successful state change.
	OnTransition func(ctx context.Context, from, to, signal string, payload T)
	// OnError is called when a Signal returns an error (state may be unchanged).
	OnError func(ctx context.Context, state, signal string, err error, payload T)
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal compiled route types (shared by workflow.go and define.go)
// ─────────────────────────────────────────────────────────────────────────────

type routeKind int

const (
	routeSimple routeKind = iota
	routeCond
	routeSwitch
)

// route is the immutable compiled form of a single signal handler.
type route[T any] struct {
	kind routeKind

	// routeSimple
	dst   string
	steps []step[T] // ordered activities; each may carry a saga compensation

	// routeCond
	condCases []condCase[T]
	elseDst   string
	hasElse   bool

	// routeSwitch
	switchExpr  SwitchExpr[T]
	switchCases []switchCase[T]
	defaultDst  string
	hasDefault  bool
}

// step represents a single Activity with an optional Saga compensation.
// When compensate is non-nil, a failure of any later step triggers this compensation.
type step[T any] struct {
	activity   Activity[T]
	compensate Activity[T] // nil = no Saga compensation
}

type condCase[T any] struct {
	cond Condition[T]
	dst  string
}

type switchCase[T any] struct {
	value any
	dst   string
}

// hookGroup holds Activities that run on state entry or exit.
// When concurrent is true all Activities in the group run in parallel goroutines.
type hookGroup[T any] struct {
	activities []Activity[T]
	concurrent bool
}
