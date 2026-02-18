package statemachine

import (
	"context"
	"errors"
)

// Sentinel errors returned by the state machine. Use errors.Is to check for them.
var (
	ErrNilModel            = errors.New("model is nil")
	ErrUndefinedTransition = errors.New("transition is not defined")
	ErrUninitializedSM     = errors.New("state machine not initialized")
	ErrDuplicateTransition = errors.New("transition already defined for this source state and event")
	ErrGuardFailed         = errors.New("guard condition failed")
	ErrNoMatchingTransition = errors.New("no matching transition found (all guards failed)")
	ErrConcurrentExecutionFailed = errors.New("concurrent execution failed")

	// ErrIgnore may be returned from an OnFailure handler to swallow the error
	// and abort the transition without changing state. The model is returned unchanged.
	ErrIgnore = errors.New("err_ignore")
)

// Error is a transition failure code passed to OnFailure handlers.
type Error string

const (
	ErrCodeUninitializedSM Error = "ERR_UNINITIALIZED_SM"
	ErrCodeNilModel        Error = "ERR_NIL_MODEL"
	ErrCodeUndefinedTransition Error = "ERR_UNDEFINED_TRANSITION"
	ErrCodeBeforeTransition    Error = "ERR_BEFORE_TRANSITION"
	ErrCodeTransition          Error = "ERR_TRANSITION"
	ErrCodeAfterTransition     Error = "ERR_AFTER_TRANSITION"
	ErrCodeGuardFailed         Error = "ERR_GUARD_FAILED"
)

// TransitionModel is the entity whose state is driven by the state machine.
// Implement SetState and GetState so the machine can read and update state.
type TransitionModel interface {
	SetState(State)
	GetState() State
}

// TransitionEvent identifies the event that triggers a transition.
type TransitionEvent interface {
	GetEvent() Event
}

// State and Event are the state and event identifiers used in transitions.
type State string
type Event string

// EventKey uniquely identifies a transition (source state + event).
type EventKey struct {
	Src   State
	Event Event
}

// Transition defines a single transition: from any of Src states, on Event, to Dst.
// Handlers are optional. If a handler returns a non-nil TransitionModel, that
// model is used for subsequent steps.
//
// Guard: If provided, the transition only executes if Guard returns true.
// This allows conditional transitions based on model state or external conditions.
// Multiple transitions with the same (state, event) are evaluated in order; the first
// matching guard wins (conditional flow chart support).
//
// Concurrent: If true, state entry/exit handlers and transition handlers run concurrently
// when possible (using goroutines). Use with caution: handlers must be thread-safe.
type Transition struct {
	Src              []State
	Event            Event
	Dst              State
	Guard            GuardHandler // Optional: condition that must be true for transition to execute
	Concurrent       bool         // If true, handlers run concurrently when possible
	BeforeTransition BeforeTransitionHandler
	Transition       TransitionHandler
	AfterTransition  AfterTransitionHandler
	OnSuccess        OnSuccessHandler
	OnFailure        OnFailureHandler
}

// GuardHandler returns true if the transition should be allowed, false otherwise.
// If Guard returns false, TriggerTransition returns ErrGuardFailed.
// Guard is evaluated before BeforeTransition.
type GuardHandler func(context.Context, TransitionEvent, TransitionModel) (bool, error)

// ConditionCase represents a single condition in an if-else or switch block.
// Condition is evaluated; if true, the associated transition executes.
type ConditionCase struct {
	Condition  GuardHandler // Evaluates to true/false
	Transition Transition   // Transition to execute if condition is true
}

// ConditionBlock represents an if-else chain: multiple conditions evaluated in order.
// First matching condition executes its transition. If ElseTransition is provided,
// it executes when all conditions fail (like an "else" clause).
type ConditionBlock struct {
	Src            []State
	Event          Event
	Cases          []ConditionCase // If-else conditions evaluated in order
	ElseTransition *Transition     // Optional: executes if all conditions fail
}

// SwitchCase represents a case in a switch block.
// Value is compared against the switch expression; if equal, transition executes.
type SwitchCase struct {
	Value      interface{} // Value to match against switch expression
	Transition Transition  // Transition to execute if value matches
}

// SwitchBlock represents a switch statement: matches a value and routes to different transitions.
// SwitchExpr extracts the value to match from the model/event.
// Cases are evaluated in order; first match executes. DefaultTransition executes if no match.
type SwitchBlock struct {
	Src               []State
	Event             Event
	SwitchExpr        func(context.Context, TransitionEvent, TransitionModel) (interface{}, error) // Extracts value to switch on
	Cases             []SwitchCase                                                                   // Cases to match against
	DefaultTransition *Transition                                                                    // Optional: executes if no case matches
}

// StateEntryHandler is called when entering a state (after state is set).
// Register with AddStateEntry or AddStateExit.
type StateEntryHandler func(context.Context, TransitionModel) error

// StateExitHandler is called when exiting a state (before state change).
// Register with AddStateEntry or AddStateExit.
type StateExitHandler func(context.Context, TransitionModel) error

// StateMachine defines a finite state machine with configurable transitions and hooks.
type StateMachine interface {
	AddTransition(Transition) error
	AddTransitions(...Transition) error
	// AddConditionBlock adds an if-else block: conditions evaluated in order, first match executes.
	// If ElseTransition is provided, it executes when all conditions fail (else clause).
	AddConditionBlock(ConditionBlock) error
	// AddSwitchBlock adds a switch block: matches a value and routes to different transitions.
	// SwitchExpr extracts the value to match; cases are evaluated in order.
	// DefaultTransition executes if no case matches (default clause).
	AddSwitchBlock(SwitchBlock) error
	// AddStateEntry registers a callback to be called when entering the given state.
	// If concurrent is true, multiple entry handlers for the same state run concurrently.
	AddStateEntry(state State, handler StateEntryHandler)
	// AddStateEntryConcurrent registers a callback that runs concurrently with other entry handlers.
	AddStateEntryConcurrent(state State, handler StateEntryHandler)
	// AddStateExit registers a callback to be called when exiting the given state.
	AddStateExit(state State, handler StateExitHandler)
	// AddStateExitConcurrent registers a callback that runs concurrently with other exit handlers.
	AddStateExitConcurrent(state State, handler StateExitHandler)
	TriggerTransition(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error)
	// TriggerParallelTransitions triggers multiple transitions concurrently from the same state.
	// All transitions must be valid for the current state. Returns the first error encountered
	// or the first successful result. Use with caution: transitions should be independent.
	TriggerParallelTransitions(context.Context, []TransitionEvent, TransitionModel) (TransitionModel, error)
	// GetTransitions returns the initial event key and a copy of all transitions.
	// The returned map contains slices of transitions (for conditional flow support).
	GetTransitions() (EventKey, map[EventKey][]Transition)
}

// Handler types for transition lifecycle hooks.
type OnSuccessHandler func(context.Context, TransitionModel) (TransitionModel, error)
type OnFailureHandler func(context.Context, TransitionModel, Error, error) (TransitionModel, error)
type TransitionHandler func(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error)
type BeforeTransitionHandler func(context.Context, TransitionModel) (TransitionModel, error)
type AfterTransitionHandler func(context.Context, TransitionModel) (TransitionModel, error)
