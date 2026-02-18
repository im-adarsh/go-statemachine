package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Workflow is the immutable, compiled state graph — the go-statemachine equivalent
// of a Temporal Workflow Definition. Build it once with Define() and share it
// freely across goroutines; all mutable per-execution state lives in Execution.
//
// Use NewExecution to create a stateful instance for a single entity.
type Workflow[T any] struct {
	routes     map[string]map[string]*route[T]
	entryHooks map[string][]hookGroup[T]
	exitHooks  map[string][]hookGroup[T]
	logger     Logger
}

// Signal drives the workflow from currentState via signal, returning the new state.
// It is stateless: the caller is responsible for persisting the returned state.
//
// Execution order for a successful transition:
//  1. Resolve destination state (evaluate conditions/switch if needed).
//  2. Log the transition.
//  3. Run OnExit hooks for currentState.
//  4. Run transition Activities (with Saga compensation on failure).
//  5. Run OnEnter hooks for the new state.
//
// On error the state is always returned alongside the error so callers know
// the effective state even when a late hook fails.
func (w *Workflow[T]) Signal(ctx context.Context, currentState, signal string, payload T) (string, error) {
	signals, ok := w.routes[currentState]
	if !ok {
		return currentState, fmt.Errorf("%w: state=%q signal=%q", ErrUnknownSignal, currentState, signal)
	}
	r, ok := signals[signal]
	if !ok {
		return currentState, fmt.Errorf("%w: state=%q signal=%q", ErrUnknownSignal, currentState, signal)
	}

	dst, err := w.resolve(ctx, r, payload)
	if err != nil {
		return currentState, err
	}

	if err := w.runHooks(ctx, w.exitHooks[currentState], payload); err != nil {
		return currentState, fmt.Errorf("OnExit hook failed for state=%q: %w", currentState, err)
	}

	if err := w.executeSteps(ctx, r, payload); err != nil {
		return currentState, err
	}

	// State is now dst; log only once the transition cannot be rolled back.
	if w.logger != nil {
		w.logger.LogTransition(currentState, signal, dst)
	}

	// OnEnter failure returns dst so callers know the state has already changed.
	if err := w.runHooks(ctx, w.entryHooks[dst], payload); err != nil {
		return dst, fmt.Errorf("OnEnter hook failed for state=%q: %w", dst, err)
	}

	return dst, nil
}

// executeSteps runs each step in order. On failure it runs Saga compensations
// in reverse for every previously-completed step that carries one.
func (w *Workflow[T]) executeSteps(ctx context.Context, r *route[T], payload T) error {
	executed := make([]int, 0, len(r.steps))
	for i, s := range r.steps {
		if err := s.activity(ctx, payload); err != nil {
			// Compensate in reverse order (best-effort; errors are suppressed).
			for j := len(executed) - 1; j >= 0; j-- {
				if comp := r.steps[executed[j]].compensate; comp != nil {
					_ = comp(ctx, payload)
				}
			}
			return fmt.Errorf("activity[%d] failed: %w", i, err)
		}
		executed = append(executed, i)
	}
	return nil
}

// resolve determines the destination state from the route definition.
func (w *Workflow[T]) resolve(ctx context.Context, r *route[T], payload T) (string, error) {
	switch r.kind {
	case routeSimple:
		return r.dst, nil

	case routeCond:
		for _, c := range r.condCases {
			if c.cond(ctx, payload) {
				return c.dst, nil
			}
		}
		if r.hasElse {
			return r.elseDst, nil
		}
		return "", ErrNoConditionMatched

	case routeSwitch:
		val := r.switchExpr(ctx, payload)
		for _, sc := range r.switchCases {
			if sc.value == val {
				return sc.dst, nil
			}
		}
		if r.hasDefault {
			return r.defaultDst, nil
		}
		return "", fmt.Errorf("%w: switch matched no case (value=%v)", ErrNoConditionMatched, val)

	default:
		return "", fmt.Errorf("unknown route kind %d", r.kind)
	}
}

// runHooks executes all hookGroups for a state entry or exit.
// Groups are executed sequentially; within a group Activities may be concurrent.
func (w *Workflow[T]) runHooks(ctx context.Context, groups []hookGroup[T], payload T) error {
	for _, g := range groups {
		if !g.concurrent {
			for _, a := range g.activities {
				if err := a(ctx, payload); err != nil {
					return err
				}
			}
			continue
		}

		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			first error
		)
		for _, a := range g.activities {
			wg.Add(1)
			go func(act Activity[T]) {
				defer wg.Done()
				if err := act(ctx, payload); err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
				}
			}(a)
		}
		wg.Wait()
		if first != nil {
			return first
		}
	}
	return nil
}

// NewExecution creates a stateful Execution that tracks the current state
// and event history for a single entity.
//
// ctx is the lifecycle context for the entire Execution; cancelling it
// prevents future Signals from running (see Execution.Cancel).
func (w *Workflow[T]) NewExecution(ctx context.Context, initialState string, opts ...ExecutionOption[T]) *Execution[T] {
	ctx, cancel := context.WithCancel(ctx)
	e := &Execution[T]{
		workflow:  w,
		state:     initialState,
		cancelCtx: ctx,
		cancelFn:  cancel,
	}
	e.cond = sync.NewCond(&e.mu)
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// AvailableSignals returns the signals accepted in state, sorted alphabetically.
func (w *Workflow[T]) AvailableSignals(state string) []string {
	sigs := make([]string, 0, len(w.routes[state]))
	for s := range w.routes[state] {
		sigs = append(sigs, s)
	}
	sort.Strings(sigs)
	return sigs
}

// States returns all states that have at least one outgoing transition, sorted.
func (w *Workflow[T]) States() []string {
	states := make([]string, 0, len(w.routes))
	for s := range w.routes {
		states = append(states, s)
	}
	sort.Strings(states)
	return states
}

// Visualize returns a human-readable text representation of the workflow graph.
func (w *Workflow[T]) Visualize() string {
	if len(w.routes) == 0 {
		return "(empty workflow)\n"
	}
	var sb strings.Builder
	sb.WriteString("Workflow:\n")
	for _, state := range w.States() {
		sb.WriteString(fmt.Sprintf("  [%s]\n", state))
		for _, sig := range w.AvailableSignals(state) {
			r := w.routes[state][sig]
			sb.WriteString(fmt.Sprintf("    --%s--> %s\n", sig, routeLabel(r)))
		}
	}
	return sb.String()
}

func routeLabel[T any](r *route[T]) string {
	switch r.kind {
	case routeSimple:
		return r.dst
	case routeCond:
		dsts := make([]string, 0, len(r.condCases)+1)
		for _, c := range r.condCases {
			dsts = append(dsts, c.dst)
		}
		if r.hasElse {
			dsts = append(dsts, r.elseDst+" (else)")
		}
		return "[" + strings.Join(dsts, " | ") + "]"
	case routeSwitch:
		dsts := make([]string, 0, len(r.switchCases)+1)
		for _, sc := range r.switchCases {
			dsts = append(dsts, fmt.Sprintf("%v→%s", sc.value, sc.dst))
		}
		if r.hasDefault {
			dsts = append(dsts, r.defaultDst+" (default)")
		}
		return "[" + strings.Join(dsts, " | ") + "]"
	default:
		return "?"
	}
}
