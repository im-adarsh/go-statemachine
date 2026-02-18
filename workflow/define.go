package workflow

import "fmt"

// ─────────────────────────────────────────────────────────────────────────────
// Builder-internal pre-compilation types
// ─────────────────────────────────────────────────────────────────────────────

// routeDef is the mutable, pre-compilation form of a route, accumulated by the
// fluent builder before Build() compiles it into the immutable route struct.
type routeDef struct {
	kind routeKind

	// routeSimple
	dst        string
	activities []Activity

	// routeCond
	condCases []condCaseDef
	elseDst   string
	hasElse   bool

	// routeSwitch
	switchExpr  SwitchExpr
	switchCases []switchCaseDef
	defaultDst  string
	hasDefault  bool
}

type condCaseDef struct {
	cond Condition
	dst  string
}

type switchCaseDef struct {
	value any
	dst   string
}

// hookDef records an OnEnter or OnExit Activity group before compilation.
type hookDef struct {
	state      string
	activities []Activity
	concurrent bool
	entry      bool // true = OnEnter, false = OnExit
}

// ─────────────────────────────────────────────────────────────────────────────
// Builder
// ─────────────────────────────────────────────────────────────────────────────

// Builder accumulates Workflow transitions and Activity hooks, then compiles
// them into an immutable Workflow via Build() or MustBuild().
// Always start with Define(); the zero value is not usable.
type Builder struct {
	routes    map[string]map[string]routeDef
	hooks     []hookDef
	logger    Logger
	buildErrs []error
}

// Define returns a new Builder — the entry point for defining a Workflow,
// analogous to writing a Temporal Workflow function.
//
//	wf, err := workflow.Define().
//	    From("PENDING").On("approve").To("APPROVED").Activity(sendApprovalEmail).
//	    From("APPROVED").On("ship").To("SHIPPED").
//	    OnEnter("SHIPPED", notifyCustomer).
//	    Build()
func Define() *Builder {
	return &Builder{routes: make(map[string]map[string]routeDef)}
}

func (b *Builder) addRoute(states []string, signal string, r routeDef) {
	for _, state := range states {
		if b.routes[state] == nil {
			b.routes[state] = make(map[string]routeDef)
		}
		if _, exists := b.routes[state][signal]; exists {
			b.buildErrs = append(b.buildErrs,
				fmt.Errorf("duplicate transition: state=%q signal=%q", state, signal))
			continue
		}
		b.routes[state][signal] = r
	}
}

// WithLogger sets the Logger used to record each executed transition.
func (b *Builder) WithLogger(l Logger) *Builder {
	b.logger = l
	return b
}

// OnEnter registers Activities called sequentially when the Execution enters state.
// Multiple OnEnter calls on the same state append additional sequential groups.
func (b *Builder) OnEnter(state string, activities ...Activity) *Builder {
	b.hooks = append(b.hooks, hookDef{state: state, activities: activities, entry: true})
	return b
}

// OnEnterParallel registers Activities that run concurrently when the Execution
// enters state. All Activities in the group are started together; the first error
// is returned after all goroutines finish.
func (b *Builder) OnEnterParallel(state string, activities ...Activity) *Builder {
	b.hooks = append(b.hooks, hookDef{state: state, activities: activities, concurrent: true, entry: true})
	return b
}

// OnExit registers Activities called sequentially when the Execution exits state.
func (b *Builder) OnExit(state string, activities ...Activity) *Builder {
	b.hooks = append(b.hooks, hookDef{state: state, activities: activities, entry: false})
	return b
}

// OnExitParallel registers Activities that run concurrently when the Execution
// exits state.
func (b *Builder) OnExitParallel(state string, activities ...Activity) *Builder {
	b.hooks = append(b.hooks, hookDef{state: state, activities: activities, concurrent: true, entry: false})
	return b
}

// From begins a transition definition for the given source state(s).
// Pass multiple states to share a single transition definition.
func (b *Builder) From(states ...string) *FromBuilder {
	return &FromBuilder{b: b, states: states}
}

// Build compiles all registered definitions into an immutable Workflow.
// Returns an error if any duplicate transitions were registered.
func (b *Builder) Build() (*Workflow, error) {
	if len(b.buildErrs) > 0 {
		return nil, b.buildErrs[0]
	}

	wf := &Workflow{
		routes:     make(map[string]map[string]*route),
		entryHooks: make(map[string][]hookGroup),
		exitHooks:  make(map[string][]hookGroup),
		logger:     b.logger,
	}

	for state, signals := range b.routes {
		wf.routes[state] = make(map[string]*route, len(signals))
		for sig, rd := range signals {
			r := &route{
				kind:       rd.kind,
				dst:        rd.dst,
				activities: rd.activities,
			}
			switch rd.kind {
			case routeCond:
				r.condCases = make([]condCase, len(rd.condCases))
				for i, c := range rd.condCases {
					r.condCases[i] = condCase{cond: c.cond, dst: c.dst}
				}
				r.elseDst = rd.elseDst
				r.hasElse = rd.hasElse
			case routeSwitch:
				r.switchExpr = rd.switchExpr
				r.switchCases = make([]switchCase, len(rd.switchCases))
				for i, sc := range rd.switchCases {
					r.switchCases[i] = switchCase{value: sc.value, dst: sc.dst}
				}
				r.defaultDst = rd.defaultDst
				r.hasDefault = rd.hasDefault
			}
			wf.routes[state][sig] = r
		}
	}

	for _, h := range b.hooks {
		g := hookGroup{activities: h.activities, concurrent: h.concurrent}
		if h.entry {
			wf.entryHooks[h.state] = append(wf.entryHooks[h.state], g)
		} else {
			wf.exitHooks[h.state] = append(wf.exitHooks[h.state], g)
		}
	}

	return wf, nil
}

// MustBuild compiles the Workflow, panicking if Build returns an error.
// Useful in package-level var blocks where the definition is statically known.
func (b *Builder) MustBuild() *Workflow {
	wf, err := b.Build()
	if err != nil {
		panic("workflow: Build() failed: " + err.Error())
	}
	return wf
}

// ─────────────────────────────────────────────────────────────────────────────
// FromBuilder — after From()
// ─────────────────────────────────────────────────────────────────────────────

// FromBuilder is returned by Builder.From. Call On() to specify the Signal.
type FromBuilder struct {
	b      *Builder
	states []string
}

// On specifies the Signal name that triggers this transition.
func (fb *FromBuilder) On(signal string) *OnBuilder {
	return &OnBuilder{b: fb.b, states: fb.states, signal: signal}
}

// ─────────────────────────────────────────────────────────────────────────────
// OnBuilder — after On()
// ─────────────────────────────────────────────────────────────────────────────

// OnBuilder is returned by FromBuilder.On. Choose the routing strategy: To, If, or Switch.
type OnBuilder struct {
	b      *Builder
	states []string
	signal string
}

// To registers a simple transition that always routes to dst.
func (ob *OnBuilder) To(dst string) *SimpleRouteBuilder {
	return &SimpleRouteBuilder{
		b:      ob.b,
		states: ob.states,
		signal: ob.signal,
		rd:     routeDef{kind: routeSimple, dst: dst},
	}
}

// If begins a Condition-based (if-else) routing chain.
// Follow with .To(dst) to specify the destination for this Condition.
func (ob *OnBuilder) If(cond Condition) *IfBuilder {
	return &IfBuilder{
		b:      ob.b,
		states: ob.states,
		signal: ob.signal,
		rd:     routeDef{kind: routeCond},
		cond:   cond,
	}
}

// Switch begins a value-based routing chain using a SwitchExpr.
// Follow with .Case(value, dst) calls and optionally .Default(dst).
func (ob *OnBuilder) Switch(expr SwitchExpr) *SwitchBuilder {
	return &SwitchBuilder{
		b:      ob.b,
		states: ob.states,
		signal: ob.signal,
		rd:     routeDef{kind: routeSwitch, switchExpr: expr},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SimpleRouteBuilder — after To()
// ─────────────────────────────────────────────────────────────────────────────

// SimpleRouteBuilder is returned after a simple To() routing call.
// Attach Activities with Activity(), then chain the next definition.
type SimpleRouteBuilder struct {
	b      *Builder
	states []string
	signal string
	rd     routeDef
}

// Activity registers one or more Activities that run sequentially during this
// transition — analogous to calling workflow.ExecuteActivity inside a Temporal
// Workflow function. Returns self for chaining.
func (rb *SimpleRouteBuilder) Activity(activities ...Activity) *SimpleRouteBuilder {
	rb.rd.activities = append(rb.rd.activities, activities...)
	return rb
}

func (rb *SimpleRouteBuilder) commit() { rb.b.addRoute(rb.states, rb.signal, rb.rd) }

// From finalises this transition and starts a new one.
func (rb *SimpleRouteBuilder) From(states ...string) *FromBuilder { rb.commit(); return rb.b.From(states...) }

// OnEnter finalises and registers sequential OnEnter Activities.
func (rb *SimpleRouteBuilder) OnEnter(state string, a ...Activity) *Builder { rb.commit(); return rb.b.OnEnter(state, a...) }

// OnEnterParallel finalises and registers concurrent OnEnter Activities.
func (rb *SimpleRouteBuilder) OnEnterParallel(state string, a ...Activity) *Builder { rb.commit(); return rb.b.OnEnterParallel(state, a...) }

// OnExit finalises and registers sequential OnExit Activities.
func (rb *SimpleRouteBuilder) OnExit(state string, a ...Activity) *Builder { rb.commit(); return rb.b.OnExit(state, a...) }

// OnExitParallel finalises and registers concurrent OnExit Activities.
func (rb *SimpleRouteBuilder) OnExitParallel(state string, a ...Activity) *Builder { rb.commit(); return rb.b.OnExitParallel(state, a...) }

// WithLogger finalises and sets the Logger.
func (rb *SimpleRouteBuilder) WithLogger(l Logger) *Builder { rb.commit(); return rb.b.WithLogger(l) }

// Build finalises and compiles the Workflow.
func (rb *SimpleRouteBuilder) Build() (*Workflow, error) { rb.commit(); return rb.b.Build() }

// MustBuild finalises and compiles, panicking on error.
func (rb *SimpleRouteBuilder) MustBuild() *Workflow { rb.commit(); return rb.b.MustBuild() }

// ─────────────────────────────────────────────────────────────────────────────
// IfBuilder — after If() / ElseIf()
// ─────────────────────────────────────────────────────────────────────────────

// IfBuilder holds a pending Condition. Call To() to set its destination state.
type IfBuilder struct {
	b      *Builder
	states []string
	signal string
	rd     routeDef
	cond   Condition
}

// To records the destination for the current Condition and returns a BranchBuilder.
func (ib *IfBuilder) To(dst string) *BranchBuilder {
	rd := ib.rd // struct copy — safe to append to condCases
	rd.condCases = append(rd.condCases, condCaseDef{cond: ib.cond, dst: dst})
	return &BranchBuilder{b: ib.b, states: ib.states, signal: ib.signal, rd: rd}
}

// ─────────────────────────────────────────────────────────────────────────────
// BranchBuilder — after If(...).To() or ElseIf(...).To()
// ─────────────────────────────────────────────────────────────────────────────

// BranchBuilder lets you add more ElseIf branches, a final Else, or finalise.
type BranchBuilder struct {
	b      *Builder
	states []string
	signal string
	rd     routeDef
}

// ElseIf adds another Condition branch to the chain.
func (bb *BranchBuilder) ElseIf(cond Condition) *IfBuilder {
	return &IfBuilder{b: bb.b, states: bb.states, signal: bb.signal, rd: bb.rd, cond: cond}
}

// Else sets the fallback destination (runs when all Conditions fail).
// Returns the Builder so you can continue defining the Workflow.
func (bb *BranchBuilder) Else(dst string) *Builder {
	rd := bb.rd
	rd.elseDst = dst
	rd.hasElse = true
	bb.b.addRoute(bb.states, bb.signal, rd)
	return bb.b
}

func (bb *BranchBuilder) commit() { bb.b.addRoute(bb.states, bb.signal, bb.rd) }

// From finalises (no Else) and starts the next transition.
func (bb *BranchBuilder) From(states ...string) *FromBuilder { bb.commit(); return bb.b.From(states...) }

// OnEnter finalises and registers sequential OnEnter Activities.
func (bb *BranchBuilder) OnEnter(state string, a ...Activity) *Builder { bb.commit(); return bb.b.OnEnter(state, a...) }

// OnEnterParallel finalises and registers concurrent OnEnter Activities.
func (bb *BranchBuilder) OnEnterParallel(state string, a ...Activity) *Builder { bb.commit(); return bb.b.OnEnterParallel(state, a...) }

// OnExit finalises and registers sequential OnExit Activities.
func (bb *BranchBuilder) OnExit(state string, a ...Activity) *Builder { bb.commit(); return bb.b.OnExit(state, a...) }

// OnExitParallel finalises and registers concurrent OnExit Activities.
func (bb *BranchBuilder) OnExitParallel(state string, a ...Activity) *Builder { bb.commit(); return bb.b.OnExitParallel(state, a...) }

// WithLogger finalises and sets the Logger.
func (bb *BranchBuilder) WithLogger(l Logger) *Builder { bb.commit(); return bb.b.WithLogger(l) }

// Build finalises and compiles the Workflow.
func (bb *BranchBuilder) Build() (*Workflow, error) { bb.commit(); return bb.b.Build() }

// MustBuild finalises and compiles, panicking on error.
func (bb *BranchBuilder) MustBuild() *Workflow { bb.commit(); return bb.b.MustBuild() }

// ─────────────────────────────────────────────────────────────────────────────
// SwitchBuilder — after Switch()
// ─────────────────────────────────────────────────────────────────────────────

// SwitchBuilder accumulates Case and Default entries for a switch-based route.
type SwitchBuilder struct {
	b      *Builder
	states []string
	signal string
	rd     routeDef
}

// Case registers a case: when SwitchExpr returns value, route to dst.
// Multiple Case calls are evaluated in registration order.
func (sb *SwitchBuilder) Case(value any, dst string) *SwitchBuilder {
	sb.rd.switchCases = append(sb.rd.switchCases, switchCaseDef{value: value, dst: dst})
	return sb
}

// Default sets the fallback destination when no Case matches.
// Returns the Builder so you can continue defining the Workflow.
func (sb *SwitchBuilder) Default(dst string) *Builder {
	sb.rd.defaultDst = dst
	sb.rd.hasDefault = true
	sb.b.addRoute(sb.states, sb.signal, sb.rd)
	return sb.b
}

func (sb *SwitchBuilder) commit() { sb.b.addRoute(sb.states, sb.signal, sb.rd) }

// From finalises (no Default) and starts the next transition.
func (sb *SwitchBuilder) From(states ...string) *FromBuilder { sb.commit(); return sb.b.From(states...) }

// OnEnter finalises and registers sequential OnEnter Activities.
func (sb *SwitchBuilder) OnEnter(state string, a ...Activity) *Builder { sb.commit(); return sb.b.OnEnter(state, a...) }

// OnEnterParallel finalises and registers concurrent OnEnter Activities.
func (sb *SwitchBuilder) OnEnterParallel(state string, a ...Activity) *Builder { sb.commit(); return sb.b.OnEnterParallel(state, a...) }

// OnExit finalises and registers sequential OnExit Activities.
func (sb *SwitchBuilder) OnExit(state string, a ...Activity) *Builder { sb.commit(); return sb.b.OnExit(state, a...) }

// OnExitParallel finalises and registers concurrent OnExit Activities.
func (sb *SwitchBuilder) OnExitParallel(state string, a ...Activity) *Builder { sb.commit(); return sb.b.OnExitParallel(state, a...) }

// WithLogger finalises and sets the Logger.
func (sb *SwitchBuilder) WithLogger(l Logger) *Builder { sb.commit(); return sb.b.WithLogger(l) }

// Build finalises and compiles the Workflow.
func (sb *SwitchBuilder) Build() (*Workflow, error) { sb.commit(); return sb.b.Build() }

// MustBuild finalises and compiles, panicking on error.
func (sb *SwitchBuilder) MustBuild() *Workflow { sb.commit(); return sb.b.MustBuild() }
