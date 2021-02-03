package statemachine

import (
	"context"
	"errors"
)

const (
	ErrUninitializedSm     Error = "ERR_UNINITIALIZED_SM"
	ErrNilModel                  = "ERR_NIL_MODEL"
	ErrUndefinedTransition       = "ERR_UNDEFINED_TRANSITION"
	ErrBeforeTransition          = "ERR_BEFORE_TRANSITION"
	ErrTransition                = "ERR_TRANSITION"
	ErrAfterTransition           = "ERR_AFTER_TRANSITION"
)

var ErrIgnore = errors.New("ERR_IGNORE")

type Error string
type State string
type Event string

type TransitionModel interface {
	SetState(State)
	GetState() State
}

type TransitionEvent interface {
	GetEvent() Event
}

type EventKey struct {
	Src   State
	Event Event
}

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

type StateMachine interface {
	AddTransition(Transition) error
	AddTransitions(...Transition) error
	TriggerTransition(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error)
	GetTransitions() (EventKey, map[EventKey]Transition)
}

type OnSuccessHandler func(context.Context, TransitionModel) (TransitionModel, error)
type OnFailureHandler func(context.Context, TransitionModel, Error, error) (TransitionModel, error)
type TransitionHandler func(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error)
type BeforeTransitionHandler func(context.Context, TransitionModel) (TransitionModel, error)
type AfterTransitionHandler func(context.Context, TransitionModel) (TransitionModel, error)
