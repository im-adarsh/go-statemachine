// Package statemachine provides a finite state machine for Go with configurable
// transitions and lifecycle hooks (before, during, after, on success, on failure).
//
// Define transitions by source state(s), event, and destination state. Trigger
// transitions with TriggerTransition(ctx, event, model). The model must
// implement TransitionModel (GetState/SetState); the event must implement
// TransitionEvent (GetEvent).
//
// Use errors.Is(err, statemachine.ErrUndefinedTransition) and similar to
// detect specific failures. Return statemachine.ErrIgnore from an OnFailure
// handler to swallow an error and abort the transition without changing state.
//
// Example:
//
//	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "A", Event: "e"})
//	sm.AddTransition(statemachine.Transition{
//	    Src: []statemachine.State{"A"}, Event: "e", Dst: "B",
//	})
//	model, err := sm.TriggerTransition(ctx, myEvent, myModel)
package statemachine

import (
	"context"
	"errors"
)

type stateMachine struct {
	startEvent  EventKey
	transitions map[EventKey]Transition
	logger      Logger
}

// Option configures a state machine at construction time.
type Option func(*stateMachine)

// WithLogger sets the logger used for transition attempts. Default is the standard log package.
func WithLogger(l Logger) Option {
	return func(s *stateMachine) {
		if l != nil {
			s.logger = l
		}
	}
}

// NewStatemachine creates a state machine with the given initial event key and default options.
func NewStatemachine(startEvent EventKey) StateMachine {
	return NewStatemachineWithOptions(startEvent)
}

// NewStatemachineWithOptions creates a state machine with the given initial event key and options.
func NewStatemachineWithOptions(startEvent EventKey, opts ...Option) StateMachine {
	sm := &stateMachine{
		startEvent:  startEvent,
		transitions: make(map[EventKey]Transition),
		logger:      defaultLogger{},
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

func (s *stateMachine) AddTransitions(ts ...Transition) error {
	if s.transitions == nil {
		return ErrUninitializedSM
	}
	for _, t := range ts {
		if err := s.AddTransition(t); err != nil {
			return err
		}
	}
	return nil
}

func (s *stateMachine) AddTransition(t Transition) error {
	if s.transitions == nil {
		return ErrUninitializedSM
	}
	for _, src := range t.Src {
		key := EventKey{Src: src, Event: t.Event}
		if _, ok := s.transitions[key]; ok {
			return ErrDuplicateTransition
		}
		s.transitions[key] = t
	}
	return nil
}

func (s *stateMachine) TriggerTransition(ctx context.Context, e TransitionEvent, t TransitionModel) (TransitionModel, error) {
	if t == nil {
		return nil, ErrNilModel
	}

	key := EventKey{Src: t.GetState(), Event: e.GetEvent()}
	tr, ok := s.transitions[key]
	if !ok {
		return nil, ErrUndefinedTransition
	}

	if s.logger != nil {
		s.logger.LogTransition(tr)
	}

	model := t

	// BeforeTransition
	if tr.BeforeTransition != nil {
		next, err := tr.BeforeTransition(ctx, model)
		if err != nil {
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeBeforeTransition, err)
				if errors.Is(err2, ErrIgnore) {
					return model, nil
				}
				if err2 != nil {
					return nil, err2
				}
				if model2 != nil {
					model = model2
				}
				return model, nil
			}
			return nil, err
		}
		if next != nil {
			model = next
		}
	}

	// Transition
	if tr.Transition != nil {
		next, err := tr.Transition(ctx, e, model)
		if err != nil {
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeTransition, err)
				if errors.Is(err2, ErrIgnore) {
					return model, nil
				}
				if err2 != nil {
					return nil, err2
				}
				if model2 != nil {
					model = model2
				}
				return model, nil
			}
			return nil, err
		}
		if next != nil {
			model = next
		}
	}

	model.SetState(tr.Dst)

	// AfterTransition
	if tr.AfterTransition != nil {
		next, err := tr.AfterTransition(ctx, model)
		if err != nil {
			// State already updated; OnFailure can decide how to handle
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeAfterTransition, err)
				if errors.Is(err2, ErrIgnore) {
					return model, nil
				}
				if err2 != nil {
					return nil, err2
				}
				if model2 != nil {
					model = model2
				}
				return model, nil
			}
			return nil, err
		}
		if next != nil {
			model = next
		}
	}

	if tr.OnSuccess != nil {
		return tr.OnSuccess(ctx, model)
	}
	return model, nil
}

// GetTransitions returns the initial event key and a copy of the transition map.
// The returned map is a shallow copy; modifying it does not affect the state machine.
func (s *stateMachine) GetTransitions() (EventKey, map[EventKey]Transition) {
	out := make(map[EventKey]Transition, len(s.transitions))
	for k, v := range s.transitions {
		out[k] = v
	}
	return s.startEvent, out
}
