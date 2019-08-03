package statemachine

import "context"

type State string
type Event string

type TransitionModel interface {
	SetState(State)
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
}

type StateMachine interface {
	AddTransition(Transition) error
	TriggerTransition(context.Context, EventKey, TransitionModel) (*TransitionModel, error)
	GetTransitions() (EventKey, map[EventKey]Transition)
}

type TransitionHandler func(context.Context, TransitionModel) error
type BeforeTransitionHandler func(context.Context, TransitionModel) error
type AfterTransitionHandler func(context.Context, TransitionModel) error
