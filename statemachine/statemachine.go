// Package statemachine provides a finite state machine for Go with configurable
// transitions, lifecycle hooks, guard conditions, and state entry/exit callbacks.
//
// Features:
//   - Transition lifecycle hooks: BeforeTransition, Transition, AfterTransition, OnSuccess, OnFailure
//   - Guard conditions: conditional transitions based on model state or external conditions
//   - State entry/exit callbacks: actions when entering or exiting states
//   - Thread-safe: safe for concurrent use with multiple goroutines
//   - Sentinel errors: use errors.Is for robust error handling
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
//	    Guard: func(ctx context.Context, e statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
//	        // Only allow transition if condition is met
//	        return someCondition(m), nil
//	    },
//	})
//	sm.AddStateEntry("B", func(ctx context.Context, m statemachine.TransitionModel) error {
//	    // Called when entering state B
//	    return nil
//	})
//	model, err := sm.TriggerTransition(ctx, myEvent, myModel)
package statemachine

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type stateMachine struct {
	mu              sync.RWMutex // Protects all fields for concurrent access
	startEvent      EventKey
	transitions     map[EventKey][]Transition // Multiple transitions per key for conditional flow
	conditionBlocks map[EventKey]ConditionBlock // If-else blocks for flow chart patterns
	switchBlocks    map[EventKey]SwitchBlock    // Switch blocks for flow chart patterns
	stateEntries    map[State][]stateEntryHandlerWithConcurrency
	stateExits      map[State][]stateExitHandlerWithConcurrency
	logger          Logger
}

type stateEntryHandlerWithConcurrency struct {
	handler    StateEntryHandler
	concurrent bool
}

type stateExitHandlerWithConcurrency struct {
	handler    StateExitHandler
	concurrent bool
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
		startEvent:      startEvent,
		transitions:     make(map[EventKey][]Transition),
		conditionBlocks: make(map[EventKey]ConditionBlock),
		switchBlocks:    make(map[EventKey]SwitchBlock),
		stateEntries:    make(map[State][]stateEntryHandlerWithConcurrency),
		stateExits:      make(map[State][]stateExitHandlerWithConcurrency),
		logger:          defaultLogger{},
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

func (s *stateMachine) AddTransitions(ts ...Transition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transitions == nil {
		return ErrUninitializedSM
	}
	for _, t := range ts {
		s.addTransitionLocked(t)
	}
	return nil
}

func (s *stateMachine) AddTransition(t Transition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transitions == nil {
		return ErrUninitializedSM
	}
	s.addTransitionLocked(t)
	return nil
}

func (s *stateMachine) addTransitionLocked(t Transition) {
	// Allow multiple transitions with same (state, event) for conditional flow
	for _, src := range t.Src {
		key := EventKey{Src: src, Event: t.Event}
		s.transitions[key] = append(s.transitions[key], t)
	}
}

func (s *stateMachine) AddStateEntry(state State, handler StateEntryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if handler != nil {
		s.stateEntries[state] = append(s.stateEntries[state], stateEntryHandlerWithConcurrency{
			handler:    handler,
			concurrent: false,
		})
	}
}

func (s *stateMachine) AddStateEntryConcurrent(state State, handler StateEntryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if handler != nil {
		s.stateEntries[state] = append(s.stateEntries[state], stateEntryHandlerWithConcurrency{
			handler:    handler,
			concurrent: true,
		})
	}
}

func (s *stateMachine) AddStateExit(state State, handler StateExitHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if handler != nil {
		s.stateExits[state] = append(s.stateExits[state], stateExitHandlerWithConcurrency{
			handler:    handler,
			concurrent: false,
		})
	}
}

func (s *stateMachine) AddStateExitConcurrent(state State, handler StateExitHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if handler != nil {
		s.stateExits[state] = append(s.stateExits[state], stateExitHandlerWithConcurrency{
			handler:    handler,
			concurrent: true,
		})
	}
}

func (s *stateMachine) AddConditionBlock(block ConditionBlock) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transitions == nil {
		return ErrUninitializedSM
	}
	for _, src := range block.Src {
		key := EventKey{Src: src, Event: block.Event}
		if _, exists := s.conditionBlocks[key]; exists {
			return fmt.Errorf("condition block already exists for state=%q event=%q", src, block.Event)
		}
		s.conditionBlocks[key] = block
	}
	return nil
}

func (s *stateMachine) AddSwitchBlock(block SwitchBlock) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transitions == nil {
		return ErrUninitializedSM
	}
	for _, src := range block.Src {
		key := EventKey{Src: src, Event: block.Event}
		if _, exists := s.switchBlocks[key]; exists {
			return fmt.Errorf("switch block already exists for state=%q event=%q", src, block.Event)
		}
		s.switchBlocks[key] = block
	}
	return nil
}

func (s *stateMachine) TriggerTransition(ctx context.Context, e TransitionEvent, t TransitionModel) (TransitionModel, error) {
	if t == nil {
		return nil, ErrNilModel
	}

	currentState := t.GetState()
	event := e.GetEvent()
	key := EventKey{Src: currentState, Event: event}

	s.mu.RLock()
	switchBlock, hasSwitch := s.switchBlocks[key]
	conditionBlock, hasCondition := s.conditionBlocks[key]
	transitions, hasTransitions := s.transitions[key]
	stateExits := s.stateExits[currentState]
	s.mu.RUnlock()

	// Priority: Switch blocks > Condition blocks > Regular transitions
	var tr *Transition

	// 1. Check switch block first (highest priority)
	if hasSwitch {
		value, err := switchBlock.SwitchExpr(ctx, e, t)
		if err != nil {
			return nil, fmt.Errorf("switch expression evaluation failed: %w", err)
		}

		// Find matching case
		for _, switchCase := range switchBlock.Cases {
			if switchCase.Value == value {
				tr = &switchCase.Transition
				break
			}
		}

		// Use default if no match
		if tr == nil && switchBlock.DefaultTransition != nil {
			tr = switchBlock.DefaultTransition
		}

		if tr != nil {
			if s.logger != nil {
				s.logger.LogTransition(*tr)
			}
			return s.executeTransition(ctx, e, t, tr, currentState, stateExits)
		}
	}

	// 2. Check condition block (if-else)
	if hasCondition {
		for _, conditionCase := range conditionBlock.Cases {
			matched, err := conditionCase.Condition(ctx, e, t)
			if err != nil {
				return nil, fmt.Errorf("condition evaluation failed: %w", err)
			}
			if matched {
				tr = &conditionCase.Transition
				break
			}
		}

		// Use else transition if no condition matched
		if tr == nil && conditionBlock.ElseTransition != nil {
			tr = conditionBlock.ElseTransition
		}

		if tr != nil {
			if s.logger != nil {
				s.logger.LogTransition(*tr)
			}
			return s.executeTransition(ctx, e, t, tr, currentState, stateExits)
		}
	}

	// 3. Check regular transitions (backward compatibility)
	if hasTransitions && len(transitions) > 0 {
		// Find first matching transition (conditional flow: evaluate guards in order)
		for i := range transitions {
			candidate := &transitions[i]
			if candidate.Guard == nil {
				// No guard means always match
				tr = candidate
				break
			}
			allowed, err := candidate.Guard(ctx, e, t)
			if err != nil {
				return nil, fmt.Errorf("guard evaluation failed: %w", err)
			}
			if allowed {
				tr = candidate
				break
			}
		}

		if tr == nil {
			// All guards failed
			// If only one transition existed, return ErrGuardFailed for backward compatibility
			if len(transitions) == 1 && transitions[0].Guard != nil {
				return nil, fmt.Errorf("%w: state=%q event=%q", ErrGuardFailed, currentState, event)
			}
			return nil, fmt.Errorf("%w: state=%q event=%q", ErrNoMatchingTransition, currentState, event)
		}
	}

	if tr == nil {
		return nil, fmt.Errorf("%w: state=%q event=%q", ErrUndefinedTransition, currentState, event)
	}

	if s.logger != nil {
		s.logger.LogTransition(*tr)
	}

	model := t
	return s.executeTransition(ctx, e, model, tr, currentState, stateExits)
}

func (s *stateMachine) executeTransition(ctx context.Context, e TransitionEvent, model TransitionModel, tr *Transition, currentState State, stateExits []stateExitHandlerWithConcurrency) (TransitionModel, error) {

	// State exit handlers (before state change) - support concurrent execution
	if err := s.runStateExitHandlers(ctx, model, stateExits, tr, currentState); err != nil {
		return nil, err
	}

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

	// Get state entry handlers for destination state
	s.mu.RLock()
	stateEntries := s.stateEntries[tr.Dst]
	s.mu.RUnlock()

	// State entry handlers (after state change) - support concurrent execution
	if err := s.runStateEntryHandlers(ctx, model, stateEntries, tr, tr.Dst); err != nil {
		return nil, err
	}

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

// runStateExitHandlers executes state exit handlers, supporting concurrent execution.
func (s *stateMachine) runStateExitHandlers(ctx context.Context, model TransitionModel, handlers []stateExitHandlerWithConcurrency, tr *Transition, state State) error {
	if len(handlers) == 0 {
		return nil
	}

	var sequentialHandlers []StateExitHandler
	var concurrentHandlers []StateExitHandler

	for _, handler := range handlers {
		if handler.concurrent {
			concurrentHandlers = append(concurrentHandlers, handler.handler)
		} else {
			sequentialHandlers = append(sequentialHandlers, handler.handler)
		}
	}
	if len(handlers) == 0 {
		return nil
	}

	// Run sequential handlers first
	for _, handler := range sequentialHandlers {
		if err := handler(ctx, model); err != nil {
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeBeforeTransition, err)
				if errors.Is(err2, ErrIgnore) {
					return nil
				}
				if err2 != nil {
					return err2
				}
				if model2 != nil {
					// Note: model update not propagated in this context, but OnFailure can handle it
				}
				return nil
			}
			return fmt.Errorf("state handler failed: state=%q: %w", state, err)
		}
	}

	// Run concurrent handlers
	if len(concurrentHandlers) > 0 {
		var wg sync.WaitGroup
		errChan := make(chan error, len(concurrentHandlers))
		for _, handler := range concurrentHandlers {
			wg.Add(1)
			go func(h func(context.Context, TransitionModel) error) {
				defer wg.Done()
				if err := h(ctx, model); err != nil {
					errChan <- err
				}
			}(handler)
		}
		wg.Wait()
		close(errChan)

		// Check for errors
		for err := range errChan {
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeBeforeTransition, err)
				if errors.Is(err2, ErrIgnore) {
					continue
				}
				if err2 != nil {
					return err2
				}
				_ = model2
				return nil
			}
			return fmt.Errorf("concurrent state handler failed: state=%q: %w", state, err)
		}
	}

	return nil
}

// runStateEntryHandlers executes state entry handlers, supporting concurrent execution.
func (s *stateMachine) runStateEntryHandlers(ctx context.Context, model TransitionModel, handlers []stateEntryHandlerWithConcurrency, tr *Transition, state State) error {
	if len(handlers) == 0 {
		return nil
	}

	var sequentialHandlers []StateEntryHandler
	var concurrentHandlers []StateEntryHandler

	for _, handler := range handlers {
		if handler.concurrent {
			concurrentHandlers = append(concurrentHandlers, handler.handler)
		} else {
			sequentialHandlers = append(sequentialHandlers, handler.handler)
		}
	}

	// Run sequential handlers first
	for _, handler := range sequentialHandlers {
		if err := handler(ctx, model); err != nil {
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeAfterTransition, err)
				if errors.Is(err2, ErrIgnore) {
					return nil
				}
				if err2 != nil {
					return err2
				}
				_ = model2
				return nil
			}
			return fmt.Errorf("state entry handler failed: state=%q: %w", state, err)
		}
	}

	// Run concurrent handlers
	if len(concurrentHandlers) > 0 {
		var wg sync.WaitGroup
		errChan := make(chan error, len(concurrentHandlers))
		for _, handler := range concurrentHandlers {
			wg.Add(1)
			go func(h StateEntryHandler) {
				defer wg.Done()
				if err := h(ctx, model); err != nil {
					errChan <- err
				}
			}(handler)
		}
		wg.Wait()
		close(errChan)

		// Check for errors
		for err := range errChan {
			if tr.OnFailure != nil {
				model2, err2 := tr.OnFailure(ctx, model, ErrCodeAfterTransition, err)
				if errors.Is(err2, ErrIgnore) {
					continue
				}
				if err2 != nil {
					return err2
				}
				_ = model2
				return nil
			}
			return fmt.Errorf("concurrent state entry handler failed: state=%q: %w", state, err)
		}
	}

	return nil
}

// TriggerParallelTransitions triggers multiple transitions concurrently from the same state.
func (s *stateMachine) TriggerParallelTransitions(ctx context.Context, events []TransitionEvent, model TransitionModel) (TransitionModel, error) {
	if model == nil {
		return nil, ErrNilModel
	}
	if len(events) == 0 {
		return model, nil
	}

	type result struct {
		model TransitionModel
		err   error
	}

	results := make(chan result, len(events))
	var wg sync.WaitGroup

	for _, evt := range events {
		wg.Add(1)
		go func(e TransitionEvent) {
			defer wg.Done()
			m, err := s.TriggerTransition(ctx, e, model)
			results <- result{model: m, err: err}
		}(evt)
	}

	wg.Wait()
	close(results)

	// Collect results - return first success or first error
	var firstErr error
	for r := range results {
		if r.err == nil && r.model != nil {
			return r.model, nil
		}
		if firstErr == nil {
			firstErr = r.err
		}
	}

	if firstErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrConcurrentExecutionFailed, firstErr)
	}
	return model, nil
}

// GetTransitions returns the initial event key and a copy of the transition map.
// The returned map contains slices of transitions (for conditional flow support).
func (s *stateMachine) GetTransitions() (EventKey, map[EventKey][]Transition) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[EventKey][]Transition, len(s.transitions))
	for k, v := range s.transitions {
		out[k] = make([]Transition, len(v))
		copy(out[k], v)
	}
	return s.startEvent, out
}
