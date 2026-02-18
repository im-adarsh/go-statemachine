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
type Transition struct {
	Src              []State
	Event            Event
	Dst              State
	BeforeTransition BeforeTransitionHandler
	Transition       TransitionHandler
	AfterTransition  AfterTransitionHandler
	OnSuccess        OnSuccessHandler
	OnFailure        OnFailureHandler
}

// StateMachine defines a finite state machine with configurable transitions and hooks.
type StateMachine interface {
	AddTransition(Transition) error
	AddTransitions(...Transition) error
	TriggerTransition(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error)
	// GetTransitions returns the initial event key and a copy of all transitions.
	// The returned map must not be modified.
	GetTransitions() (EventKey, map[EventKey]Transition)
}

// Handler types for transition lifecycle hooks.
type OnSuccessHandler func(context.Context, TransitionModel) (TransitionModel, error)
type OnFailureHandler func(context.Context, TransitionModel, Error, error) (TransitionModel, error)
type TransitionHandler func(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error)
type BeforeTransitionHandler func(context.Context, TransitionModel) (TransitionModel, error)
type AfterTransitionHandler func(context.Context, TransitionModel) (TransitionModel, error)
