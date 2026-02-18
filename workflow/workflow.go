package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Workflow is an immutable, compiled state machine definition — analogous to a
// Temporal Workflow Definition. Build one with Define().Build() or
// Define().MustBuild(), then share it freely across goroutines.
//
// The Workflow itself holds no state. Call NewExecution to create a stateful
// Execution, or call Signal directly for a fully stateless interaction.
type Workflow struct {
	routes     map[string]map[string]*route // state → signal → compiled route
	entryHooks map[string][]hookGroup       // state → ordered OnEnter Activity groups
	exitHooks  map[string][]hookGroup       // state → ordered OnExit Activity groups
	logger     Logger
}

// Signal is the stateless API. It resolves the destination state and runs all
// Activities for the given (currentState, signal) pair, then returns the new state.
//
// Execution order:
//  1. Resolve destination (evaluate Conditions / SwitchExpr)
//  2. Log the transition
//  3. Run OnExit Activities for currentState (sequential groups, then parallel groups)
//  4. Run transition Activities (sequential)
//  5. Run OnEnter Activities for the destination state
//  6. Return the new state
//
// On any error before state change the original state is returned unchanged.
// An OnEnter failure returns the new state alongside the error because the
// transition already resolved — callers may choose to treat this as a partial success.
func (w *Workflow) Signal(ctx context.Context, currentState, signal string, payload any) (string, error) {
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

	if w.logger != nil {
		w.logger.LogTransition(currentState, signal, dst)
	}

	if err := w.runHooks(ctx, w.exitHooks[currentState], payload); err != nil {
		return currentState, fmt.Errorf("OnExit activity failed for state=%q: %w", currentState, err)
	}

	for _, a := range r.activities {
		if err := a(ctx, payload); err != nil {
			return currentState, fmt.Errorf("transition activity failed: %w", err)
		}
	}

	if err := w.runHooks(ctx, w.entryHooks[dst], payload); err != nil {
		return dst, fmt.Errorf("OnEnter activity failed for state=%q: %w", dst, err)
	}

	return dst, nil
}

// resolve determines the destination state from the compiled route.
func (w *Workflow) resolve(ctx context.Context, r *route, payload any) (string, error) {
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

// runHooks executes the ordered hookGroups for a state. Sequential groups run
// one Activity at a time; concurrent groups fan out with goroutines.
func (w *Workflow) runHooks(ctx context.Context, groups []hookGroup, payload any) error {
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
			go func(act Activity) {
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

// NewExecution creates a new Execution backed by this Workflow, starting in
// initialState — analogous to starting a new Temporal Workflow Execution.
func (w *Workflow) NewExecution(initialState string) *Execution {
	return &Execution{workflow: w, state: initialState}
}

// AvailableSignals returns the signals that can be received in the given state,
// sorted alphabetically.
func (w *Workflow) AvailableSignals(state string) []string {
	sigs := make([]string, 0, len(w.routes[state]))
	for s := range w.routes[state] {
		sigs = append(sigs, s)
	}
	sort.Strings(sigs)
	return sigs
}

// States returns all states that have at least one outgoing signal registered,
// sorted alphabetically.
func (w *Workflow) States() []string {
	states := make([]string, 0, len(w.routes))
	for s := range w.routes {
		states = append(states, s)
	}
	sort.Strings(states)
	return states
}

// Visualize returns a human-readable diagram of the Workflow's state transitions.
func (w *Workflow) Visualize() string {
	if len(w.routes) == 0 {
		return "(empty workflow)"
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

func routeLabel(r *route) string {
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
	}
	return "?"
}
