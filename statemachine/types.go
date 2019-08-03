package statemachine

import (
	"context"
)

const (
	ERR_UNINITIALIZED_SM     StateMachineError = "ERR_UNINITIALIZED_SM"
	ERR_NIL_MODEL                              = "ERR_NIL_MODEL"
	ERR_UNDEFINED_TRANSITION                   = "ERR_UNDEFINED_TRANSITION"
	ERR_BEFORE_TRANSITION                      = "ERR_BEFORE_TRANSITION"
	ERR_TRANSITION                             = "ERR_TRANSITION"
	ERR_AFTER_TRANSITION                       = "ERR_AFTER_TRANSITION"
)

type StateMachineError string
type State string
type Event string

type TransitionModel interface {
	SetState(State)
	GetState() State
}

type EventKey struct {
	Src   State
	Event Event
}

type Transition struct {
	Src              State
	Event            Event
	Dst              State
	BeforeTransition BeforeTransitionHandler
	Transition       TransitionHandler
	AfterTransition  AfterTransitionHandler
	OnSucess         OnSucessHandler
	OnFailure        OnFailureHandler
}

type StateMachine interface {
	AddTransition(Transition) error
	TriggerTransition(context.Context, Event, TransitionModel) error
	GetTransitions() (EventKey, map[EventKey]Transition)
}

type OnSucessHandler func(context.Context, TransitionModel) error
type OnFailureHandler func(context.Context, TransitionModel, StateMachineError, error) error
type TransitionHandler func(context.Context, TransitionModel) error
type BeforeTransitionHandler func(context.Context, TransitionModel) error
type AfterTransitionHandler func(context.Context, TransitionModel) error
