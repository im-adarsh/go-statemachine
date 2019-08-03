package statemachine

import (
	"context"
	"errors"
	"fmt"
)

type stateMachine struct {
	startEvent  EventKey
	transitions map[EventKey]Transition
}

func NewStatemachine(startEvent EventKey) StateMachine {
	transitions := map[EventKey]Transition{}
	return &stateMachine{
		startEvent:  startEvent,
		transitions: transitions,
	}
}

func (s *stateMachine) AddTransition(t Transition) error {

	if s.transitions == nil {
		return errors.New("transition is not added")
	}

	if _, ok := s.transitions[EventKey{Src: t.Src, Event: t.Event}]; ok {
		return errors.New("transition is already present")
	}

	s.transitions[EventKey{Src: t.Src, Event: t.Event}] = t
	return nil
}

func (s *stateMachine) TriggerTransition(ctx context.Context, e EventKey, t TransitionModel) error {
	tr, ok := s.transitions[EventKey{Src: e.Src, Event: e.Event}]
	if !ok {
		return errors.New("transition is not found")
	}
	err := tr.BeforeTransition(ctx, t)
	if err != nil {
		return errors.New(fmt.Sprintf("before transition failed : %v", err))
	}
	err = tr.Transition(ctx, t)
	if err != nil {
		return errors.New(fmt.Sprintf("transition failed : %v", err))
	}
	t.SetState(tr.Dst)
	err = tr.AfterTransition(ctx, t)
	if err != nil {
		return errors.New(fmt.Sprintf("after transition failed : %v", err))
	}

	return nil
}

func (s *stateMachine) GetTransitions() (EventKey, map[EventKey]Transition) {
	return s.startEvent, s.transitions
}
