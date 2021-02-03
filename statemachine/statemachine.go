package statemachine

import (
	"context"
	"errors"
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

	for _, src := range t.Src {
		if _, ok := s.transitions[EventKey{Src: src, Event: t.Event}]; ok {
			return errors.New("transition is already present")
		}

		s.transitions[EventKey{Src: src, Event: t.Event}] = t
	}
	return nil
}

func (s *stateMachine) TriggerTransition(ctx context.Context, e TransitionEvent, t TransitionModel) error {

	if t == nil {
		return errors.New("model is nil")
	}

	tr, ok := s.transitions[EventKey{Src: t.GetState(), Event: e.GetEvent()}]
	if !ok {
		err := errors.New("transition is not defined")
		if tr.OnFailure == nil {
			return err
		}
		err = tr.OnFailure(ctx, t, ErrUndefinedTransition, err)
		if err != ErrIgnore {
			return err
		}
	}

	logTrigger(tr)

	if tr.BeforeTransition != nil {
		err := tr.BeforeTransition(ctx, t)
		if err != nil {
			if tr.OnFailure == nil {
				return err
			}
			err = tr.OnFailure(ctx, t, ErrBeforeTransition, err)
			if err != ErrIgnore {
				return err
			}
		}
	}

	if tr.Transition != nil {
		err := tr.Transition(ctx, e, t)
		if err != nil {
			if tr.OnFailure == nil {
				return err
			}
			err = tr.OnFailure(ctx, t, ErrTransition, err)
			if err != ErrIgnore {
				return err
			}
		}

	}

	t.SetState(tr.Dst)

	if tr.AfterTransition != nil {
		err := tr.AfterTransition(ctx, t)
		if err != nil {
			if tr.OnFailure == nil {
				return err
			}
			err = tr.OnFailure(ctx, t, ErrAfterTransition, err)
			if err != ErrIgnore {
				return err
			}
		}
	}

	if tr.OnSuccess != nil {
		return tr.OnSuccess(ctx, t)
	}

	return nil
}

func (s *stateMachine) GetTransitions() (EventKey, map[EventKey]Transition) {
	return s.startEvent, s.transitions
}
