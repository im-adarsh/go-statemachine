// Package workflow provides a stateless, fluent state machine for Go modelled
// after Temporal's concepts: Workflows, Executions, Signals, and Activities.
//
// # Terminology
//
//   - Workflow    — immutable, compiled state graph (build once, share freely).
//   - Execution   — lightweight stateful wrapper around a Workflow; one per entity.
//   - Signal      — the named event that drives an Execution to its next state.
//   - Activity    — a unit of work (func) run during transitions or on state entry/exit.
//   - Condition   — a predicate used for if-else routing.
//   - SwitchExpr  — extracts a routing key for switch-case routing.
package workflow

import "context"

// ─────────────────────────────────────────────────────────────────────────────
// Public functional types
// ─────────────────────────────────────────────────────────────────────────────

// Activity is the fundamental unit of work — the go-statemachine equivalent of a
// Temporal Activity function. It is used for:
//   - Transition activities (.Activity(...) on a route)
//   - State lifecycle hooks (OnEnter / OnExit / their parallel variants)
//
// If an Activity returns an error during a transition the signal is aborted and
// the state is unchanged. OnExit failures abort before any state change; OnEnter
// failures report the new state alongside the error.
type Activity func(ctx context.Context, payload any) error

// Condition evaluates a predicate for if-else routing.
// Return true to select the associated destination state.
type Condition func(ctx context.Context, payload any) bool

// SwitchExpr extracts the routing key from the payload/context for switch-case routing.
// The returned value is compared with Case values using ==.
type SwitchExpr func(ctx context.Context, payload any) any

// ─────────────────────────────────────────────────────────────────────────────
// Internal compiled route types (shared by workflow.go and define.go)
// ─────────────────────────────────────────────────────────────────────────────

type routeKind int

const (
	routeSimple routeKind = iota // always routes to a fixed destination
	routeCond                    // if-else chain of Conditions
	routeSwitch                  // SwitchExpr matched against Case values
)

// route is the compiled, immutable representation of a single signal handler.
type route struct {
	kind routeKind

	// routeSimple
	dst        string
	activities []Activity

	// routeCond
	condCases []condCase
	elseDst   string
	hasElse   bool

	// routeSwitch
	switchExpr  SwitchExpr
	switchCases []switchCase
	defaultDst  string
	hasDefault  bool
}

type condCase struct {
	cond Condition
	dst  string
}

type switchCase struct {
	value any
	dst   string
}

// hookGroup holds a set of Activities that run together on state entry or exit.
// When concurrent is true the Activities in the group run in parallel goroutines.
type hookGroup struct {
	activities []Activity
	concurrent bool
}
