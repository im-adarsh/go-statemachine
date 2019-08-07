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

	if _, ok := s.transitions[EventKey{Src: t.Src, Event: t.Event}]; ok {
		return errors.New("transition is already present")
	}

	s.transitions[EventKey{Src: t.Src, Event: t.Event}] = t
	return nil
}

func (s *stateMachine) TriggerTransition(ctx context.Context, e Event, t TransitionModel) error {

	if t == nil {
		return errors.New("model is nil")
	}

	currentState := t.GetState()
	if currentState == "" {
		return errors.New("currentState is nil")
	}

	tr, ok := s.transitions[EventKey{Src: t.GetState(), Event: e}]
	if !ok {
		err := errors.New("transition is not defined")
		if tr.OnFailure == nil {
			return err
		}
		err = tr.OnFailure(ctx, t, ERR_UNDEFINED_TRANSITION, err)
		if err != ERR_IGNORE {
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
			err = tr.OnFailure(ctx, t, ERR_BEFORE_TRANSITION, err)
			if err != ERR_IGNORE {
				return err
			}
		}
	}

	if tr.Transition != nil {
		err := tr.Transition(ctx, t)
		if err != nil {
			if tr.OnFailure == nil {
				return err
			}
			err = tr.OnFailure(ctx, t, ERR_TRANSITION, err)
			if err != ERR_IGNORE {
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
			err = tr.OnFailure(ctx, t, ERR_AFTER_TRANSITION, err)
			if err != ERR_IGNORE {
				return err
			}
		}
	}

	if tr.OnSucess != nil {
		return tr.OnSucess(ctx, t)
	}

	return nil
}

func (s *stateMachine) GetTransitions() (EventKey, map[EventKey]Transition) {
	return s.startEvent, s.transitions
}
