package workflow

import "fmt"

// ─────────────────────────────────────────────────────────────────────────────
// Internal builder state (mutable, per-build)
// ─────────────────────────────────────────────────────────────────────────────

type routeDef[T any] struct {
	kind routeKind

	// routeSimple
	dst   string
	steps []stepDef[T]

	// routeCond
	condCases []condCaseDef[T]
	elseDst   string
	hasElse   bool

	// routeSwitch
	switchExpr  SwitchExpr[T]
	switchCases []switchCaseDef[T]
	defaultDst  string
	hasDefault  bool
}

type stepDef[T any] struct {
	activity   Activity[T]
	compensate Activity[T] // nil = not a Saga step
}

type condCaseDef[T any] struct {
	cond Condition[T]
	dst  string
}

type switchCaseDef[T any] struct {
	value any
	dst   string
}

type hookDef[T any] struct {
	state      string
	activities []Activity[T]
	concurrent bool
	entry      bool // true = OnEnter, false = OnExit
}

// ─────────────────────────────────────────────────────────────────────────────
// Builder
// ─────────────────────────────────────────────────────────────────────────────

// Builder accumulates the workflow definition before compiling it with Build.
// Obtain one via Define[T]().
type Builder[T any] struct {
	routes     map[string]map[string]*routeDef[T]
	hooks      []hookDef[T]
	middleware []Middleware[T]
	logger     Logger
	buildErrs  []error
}

// Define returns a new Builder for a Workflow parameterised on T.
// T is the payload type passed through every Signal, Activity, and Condition.
//
//	wf, err := workflow.Define[*Order]().
//	    From("PENDING").On("approve").To("APPROVED").
//	    Build()
func Define[T any]() *Builder[T] {
	return &Builder[T]{
		routes: make(map[string]map[string]*routeDef[T]),
	}
}

// WithLogger sets a custom Logger. Defaults to NoopLogger.
func (b *Builder[T]) WithLogger(l Logger) *Builder[T] {
	b.logger = l
	return b
}

// WithMiddleware registers workflow-wide middleware applied to every Activity
// (transitions and hooks alike) at Build time. Middleware is applied in the
// order given, with the first element being the outermost wrapper.
//
//	.WithMiddleware(otelMiddleware, metricsMiddleware)
func (b *Builder[T]) WithMiddleware(mw ...Middleware[T]) *Builder[T] {
	b.middleware = append(b.middleware, mw...)
	return b
}

// OnEnter registers Activities to run when the workflow enters state.
// Multiple calls to OnEnter for the same state are allowed and appended in order.
func (b *Builder[T]) OnEnter(state string, activities ...Activity[T]) *Builder[T] {
	b.hooks = append(b.hooks, hookDef[T]{state: state, activities: activities, entry: true})
	return b
}

// OnEnterConcurrent registers Activities that run in parallel when the workflow
// enters state. It is treated as a single hookGroup.
func (b *Builder[T]) OnEnterConcurrent(state string, activities ...Activity[T]) *Builder[T] {
	b.hooks = append(b.hooks, hookDef[T]{state: state, activities: activities, concurrent: true, entry: true})
	return b
}

// OnExit registers Activities to run when the workflow leaves state.
func (b *Builder[T]) OnExit(state string, activities ...Activity[T]) *Builder[T] {
	b.hooks = append(b.hooks, hookDef[T]{state: state, activities: activities, entry: false})
	return b
}

// OnExitConcurrent registers Activities that run in parallel when the workflow
// leaves state.
func (b *Builder[T]) OnExitConcurrent(state string, activities ...Activity[T]) *Builder[T] {
	b.hooks = append(b.hooks, hookDef[T]{state: state, activities: activities, concurrent: true, entry: false})
	return b
}

// From starts a transition definition for one or more source states.
func (b *Builder[T]) From(states ...string) *FromBuilder[T] {
	if len(states) == 0 {
		b.buildErrs = append(b.buildErrs, fmt.Errorf("From() called with no states"))
	}
	return &FromBuilder[T]{b: b, states: states}
}

// Build compiles all definitions into an immutable Workflow.
// Returns an error if any duplicate or conflicting definitions were registered.
func (b *Builder[T]) Build() (*Workflow[T], error) {
	if len(b.buildErrs) > 0 {
		return nil, b.buildErrs[0]
	}

	wf := &Workflow[T]{
		routes:     make(map[string]map[string]*route[T]),
		entryHooks: make(map[string][]hookGroup[T]),
		exitHooks:  make(map[string][]hookGroup[T]),
		logger:     b.logger,
	}

	// Compile routes.
	for state, signals := range b.routes {
		wf.routes[state] = make(map[string]*route[T])
		for sig, def := range signals {
			r := &route[T]{
				kind:        def.kind,
				dst:         def.dst,
				steps:       make([]step[T], len(def.steps)),
				elseDst:     def.elseDst,
				hasElse:     def.hasElse,
				switchExpr:  def.switchExpr,
				defaultDst:  def.defaultDst,
				hasDefault:  def.hasDefault,
			}
			for i, s := range def.steps {
				r.steps[i] = step[T]{
					activity:   b.applyMiddleware(s.activity),
					compensate: b.applyMiddlewareNilable(s.compensate),
				}
			}
			// Compile condition cases.
			r.condCases = make([]condCase[T], len(def.condCases))
			for i, c := range def.condCases {
				r.condCases[i] = condCase[T]{cond: c.cond, dst: c.dst}
			}
			// Compile switch cases.
			r.switchCases = make([]switchCase[T], len(def.switchCases))
			for i, sc := range def.switchCases {
				r.switchCases[i] = switchCase[T]{value: sc.value, dst: sc.dst}
			}
			wf.routes[state][sig] = r
		}
	}

	// Compile hooks.
	for _, hd := range b.hooks {
		acts := make([]Activity[T], len(hd.activities))
		for i, a := range hd.activities {
			acts[i] = b.applyMiddleware(a)
		}
		g := hookGroup[T]{activities: acts, concurrent: hd.concurrent}
		if hd.entry {
			wf.entryHooks[hd.state] = append(wf.entryHooks[hd.state], g)
		} else {
			wf.exitHooks[hd.state] = append(wf.exitHooks[hd.state], g)
		}
	}

	return wf, nil
}

// MustBuild calls Build and panics on error. Useful for package-level vars.
func (b *Builder[T]) MustBuild() *Workflow[T] {
	wf, err := b.Build()
	if err != nil {
		panic("workflow.MustBuild: " + err.Error())
	}
	return wf
}

// applyMiddleware wraps a with the builder's workflow-wide middleware.
func (b *Builder[T]) applyMiddleware(a Activity[T]) Activity[T] {
	if len(b.middleware) == 0 {
		return a
	}
	result := a
	for i := len(b.middleware) - 1; i >= 0; i-- {
		result = b.middleware[i](result)
	}
	return result
}

// applyMiddlewareNilable wraps a only when a is non-nil.
func (b *Builder[T]) applyMiddlewareNilable(a Activity[T]) Activity[T] {
	if a == nil {
		return nil
	}
	return b.applyMiddleware(a)
}

// ensureRoute returns (or creates) the routeDef for state+signal, recording
// a build error if the combination was already defined.
func (b *Builder[T]) ensureRoute(state, signal string) *routeDef[T] {
	if b.routes[state] == nil {
		b.routes[state] = make(map[string]*routeDef[T])
	}
	if _, exists := b.routes[state][signal]; exists {
		b.buildErrs = append(b.buildErrs, fmt.Errorf("duplicate transition: state=%q signal=%q", state, signal))
		return &routeDef[T]{} // dummy to avoid nil dereferences
	}
	def := &routeDef[T]{}
	b.routes[state][signal] = def
	return def
}

// ─────────────────────────────────────────────────────────────────────────────
// FromBuilder
// ─────────────────────────────────────────────────────────────────────────────

// FromBuilder narrows the definition to a set of source states.
type FromBuilder[T any] struct {
	b      *Builder[T]
	states []string
}

// On specifies the signal that triggers the transition.
func (fb *FromBuilder[T]) On(signal string) *OnBuilder[T] {
	return &OnBuilder[T]{b: fb.b, states: fb.states, signal: signal}
}

// ─────────────────────────────────────────────────────────────────────────────
// OnBuilder
// ─────────────────────────────────────────────────────────────────────────────

// OnBuilder has selected source states and a signal; choose a routing strategy.
type OnBuilder[T any] struct {
	b      *Builder[T]
	states []string
	signal string
}

// To registers a simple (unconditional) transition to dst.
func (ob *OnBuilder[T]) To(dst string) *SimpleRouteBuilder[T] {
	defs := make([]*routeDef[T], 0, len(ob.states))
	for _, s := range ob.states {
		def := ob.b.ensureRoute(s, ob.signal)
		def.kind = routeSimple
		def.dst = dst
		defs = append(defs, def)
	}
	return &SimpleRouteBuilder[T]{b: ob.b, defs: defs}
}

// If starts a conditional (if-else) routing chain.
func (ob *OnBuilder[T]) If(cond Condition[T], dst string) *IfBuilder[T] {
	defs := make([]*routeDef[T], 0, len(ob.states))
	for _, s := range ob.states {
		def := ob.b.ensureRoute(s, ob.signal)
		def.kind = routeCond
		def.condCases = append(def.condCases, condCaseDef[T]{cond: cond, dst: dst})
		defs = append(defs, def)
	}
	return &IfBuilder[T]{b: ob.b, defs: defs}
}

// Switch starts a switch-case routing chain.
func (ob *OnBuilder[T]) Switch(expr SwitchExpr[T]) *SwitchBuilder[T] {
	defs := make([]*routeDef[T], 0, len(ob.states))
	for _, s := range ob.states {
		def := ob.b.ensureRoute(s, ob.signal)
		def.kind = routeSwitch
		def.switchExpr = expr
		defs = append(defs, def)
	}
	return &SwitchBuilder[T]{b: ob.b, defs: defs}
}

// ─────────────────────────────────────────────────────────────────────────────
// SimpleRouteBuilder
// ─────────────────────────────────────────────────────────────────────────────

// SimpleRouteBuilder configures a simple (unconditional) transition.
type SimpleRouteBuilder[T any] struct {
	b    *Builder[T]
	defs []*routeDef[T]
}

// Activity registers one or more Activities to execute during this transition.
// Activities run sequentially in the order given. For Saga compensation use Saga.
func (rb *SimpleRouteBuilder[T]) Activity(activities ...Activity[T]) *SimpleRouteBuilder[T] {
	for _, a := range activities {
		for _, def := range rb.defs {
			def.steps = append(def.steps, stepDef[T]{activity: a})
		}
	}
	return rb
}

// Saga registers an Activity paired with a compensation Activity.
// If a subsequent step fails, completed Saga compensations run in reverse order.
//
//	.Saga(chargeCard, refundCard).
//	.Saga(deductInventory, restoreInventory)
func (rb *SimpleRouteBuilder[T]) Saga(activity, compensate Activity[T]) *SimpleRouteBuilder[T] {
	for _, def := range rb.defs {
		def.steps = append(def.steps, stepDef[T]{activity: activity, compensate: compensate})
	}
	return rb
}

// From is a shortcut to start a new transition from the same Builder.
func (rb *SimpleRouteBuilder[T]) From(states ...string) *FromBuilder[T] {
	return rb.b.From(states...)
}

// OnEnter is a shortcut to add a state entry hook.
func (rb *SimpleRouteBuilder[T]) OnEnter(state string, activities ...Activity[T]) *Builder[T] {
	return rb.b.OnEnter(state, activities...)
}

// OnExit is a shortcut to add a state exit hook.
func (rb *SimpleRouteBuilder[T]) OnExit(state string, activities ...Activity[T]) *Builder[T] {
	return rb.b.OnExit(state, activities...)
}

// Build compiles and returns the Workflow.
func (rb *SimpleRouteBuilder[T]) Build() (*Workflow[T], error) {
	return rb.b.Build()
}

// MustBuild panics on error.
func (rb *SimpleRouteBuilder[T]) MustBuild() *Workflow[T] {
	return rb.b.MustBuild()
}

// ─────────────────────────────────────────────────────────────────────────────
// IfBuilder / BranchBuilder
// ─────────────────────────────────────────────────────────────────────────────

// IfBuilder continues the if-else conditional routing chain.
type IfBuilder[T any] struct {
	b    *Builder[T]
	defs []*routeDef[T]
}

// ElseIf adds another condition branch.
func (ib *IfBuilder[T]) ElseIf(cond Condition[T], dst string) *IfBuilder[T] {
	for _, def := range ib.defs {
		def.condCases = append(def.condCases, condCaseDef[T]{cond: cond, dst: dst})
	}
	return ib
}

// Else sets the fallback destination when no condition matches.
func (ib *IfBuilder[T]) Else(dst string) *BranchBuilder[T] {
	for _, def := range ib.defs {
		def.elseDst = dst
		def.hasElse = true
	}
	return &BranchBuilder[T]{b: ib.b}
}

// Build compiles and returns the Workflow (no Else required).
func (ib *IfBuilder[T]) Build() (*Workflow[T], error) {
	return ib.b.Build()
}

// MustBuild panics on error.
func (ib *IfBuilder[T]) MustBuild() *Workflow[T] {
	return ib.b.MustBuild()
}

// From is a shortcut.
func (ib *IfBuilder[T]) From(states ...string) *FromBuilder[T] {
	return ib.b.From(states...)
}

// BranchBuilder is returned after Else() or Default() to continue the definition.
type BranchBuilder[T any] struct {
	b *Builder[T]
}

// From is a shortcut.
func (bb *BranchBuilder[T]) From(states ...string) *FromBuilder[T] {
	return bb.b.From(states...)
}

// OnEnter is a shortcut.
func (bb *BranchBuilder[T]) OnEnter(state string, activities ...Activity[T]) *Builder[T] {
	return bb.b.OnEnter(state, activities...)
}

// OnExit is a shortcut.
func (bb *BranchBuilder[T]) OnExit(state string, activities ...Activity[T]) *Builder[T] {
	return bb.b.OnExit(state, activities...)
}

// Build compiles and returns the Workflow.
func (bb *BranchBuilder[T]) Build() (*Workflow[T], error) {
	return bb.b.Build()
}

// MustBuild panics on error.
func (bb *BranchBuilder[T]) MustBuild() *Workflow[T] {
	return bb.b.MustBuild()
}

// ─────────────────────────────────────────────────────────────────────────────
// SwitchBuilder
// ─────────────────────────────────────────────────────────────────────────────

// SwitchBuilder constructs a switch-case routing chain.
type SwitchBuilder[T any] struct {
	b    *Builder[T]
	defs []*routeDef[T]
}

// Case adds a value-to-destination mapping.
func (sb *SwitchBuilder[T]) Case(value any, dst string) *SwitchBuilder[T] {
	for _, def := range sb.defs {
		def.switchCases = append(def.switchCases, switchCaseDef[T]{value: value, dst: dst})
	}
	return sb
}

// Default sets the fallback destination when no case matches.
func (sb *SwitchBuilder[T]) Default(dst string) *BranchBuilder[T] {
	for _, def := range sb.defs {
		def.defaultDst = dst
		def.hasDefault = true
	}
	return &BranchBuilder[T]{b: sb.b}
}

// Build compiles and returns the Workflow (no Default required).
func (sb *SwitchBuilder[T]) Build() (*Workflow[T], error) {
	return sb.b.Build()
}

// MustBuild panics on error.
func (sb *SwitchBuilder[T]) MustBuild() *Workflow[T] {
	return sb.b.MustBuild()
}

// From is a shortcut.
func (sb *SwitchBuilder[T]) From(states ...string) *FromBuilder[T] {
	return sb.b.From(states...)
}
