package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/im-adarsh/go-statemachine/workflow"
)

var ctx = context.Background()

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

// buildable covers Builder, SimpleRouteBuilder, BranchBuilder, SwitchBuilder —
// any type returned by the fluent chain that can produce a Workflow.
type buildable interface {
	Build() (*workflow.Workflow, error)
}

func mustBuild(t *testing.T, b buildable) *workflow.Workflow {
	t.Helper()
	wf, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	return wf
}

func activityCounter(n *int32) workflow.Activity {
	return func(_ context.Context, _ any) error {
		atomic.AddInt32(n, 1)
		return nil
	}
}

// ─────────────────────────────────────────────────────────────
// Simple transitions
// ─────────────────────────────────────────────────────────────

func TestSimpleTransition(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B"))

	got, err := wf.Signal(ctx, "A", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "B" {
		t.Errorf("state = %q, want %q", got, "B")
	}
}

func TestMultipleTransitions(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("SOLID").On("melt").To("LIQUID").
		From("LIQUID").On("vaporize").To("GAS").
		From("GAS").On("condense").To("LIQUID").
		From("LIQUID").On("freeze").To("SOLID"))

	steps := []struct{ from, signal, want string }{
		{"SOLID", "melt", "LIQUID"},
		{"LIQUID", "vaporize", "GAS"},
		{"GAS", "condense", "LIQUID"},
		{"LIQUID", "freeze", "SOLID"},
	}
	for _, s := range steps {
		got, err := wf.Signal(ctx, s.from, s.signal, nil)
		if err != nil {
			t.Fatalf("Signal(%q,%q): %v", s.from, s.signal, err)
		}
		if got != s.want {
			t.Errorf("Signal(%q,%q) = %q, want %q", s.from, s.signal, got, s.want)
		}
	}
}

func TestSharedTransition_MultipleSourceStates(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A", "B", "C").On("reset").To("IDLE"))

	for _, src := range []string{"A", "B", "C"} {
		got, err := wf.Signal(ctx, src, "reset", nil)
		if err != nil {
			t.Fatalf("Signal(%q, reset): %v", src, err)
		}
		if got != "IDLE" {
			t.Errorf("from %q: state = %q, want IDLE", src, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────
// Unknown signal / state
// ─────────────────────────────────────────────────────────────

func TestUnknownSignal(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B"))

	_, err := wf.Signal(ctx, "A", "fly", nil)
	if !errors.Is(err, workflow.ErrUnknownSignal) {
		t.Fatalf("want ErrUnknownSignal, got %v", err)
	}
}

func TestUnknownState(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B"))

	_, err := wf.Signal(ctx, "Z", "go", nil)
	if !errors.Is(err, workflow.ErrUnknownSignal) {
		t.Fatalf("want ErrUnknownSignal, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────
// Duplicate signal detection
// ─────────────────────────────────────────────────────────────

func TestDuplicateTransition_BuildError(t *testing.T) {
	_, err := workflow.Define().
		From("A").On("go").To("B").
		From("A").On("go").To("C").
		Build()

	if err == nil {
		t.Fatal("expected Build() error for duplicate transition")
	}
}

// ─────────────────────────────────────────────────────────────
// Transition Activities
// ─────────────────────────────────────────────────────────────

func TestActivity_ExecutedOnce(t *testing.T) {
	var n int32
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").Activity(activityCounter(&n)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if n != 1 {
		t.Errorf("activity called %d times, want 1", n)
	}
}

func TestActivity_MultipleActivities_Sequential(t *testing.T) {
	var order []int
	makeActivity := func(i int) workflow.Activity {
		return func(_ context.Context, _ any) error {
			order = append(order, i)
			return nil
		}
	}
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		Activity(makeActivity(1), makeActivity(2), makeActivity(3)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("activities in wrong order: %v", order)
	}
}

func TestActivity_ErrorAbortsTransition(t *testing.T) {
	sentinel := errors.New("activity failed")
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").Activity(func(_ context.Context, _ any) error {
			return sentinel
		}))

	state, err := wf.Signal(ctx, "A", "go", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
	if state != "A" {
		t.Errorf("state should be unchanged on error, got %q", state)
	}
}

func TestActivity_PayloadPassedThrough(t *testing.T) {
	type payload struct{ Value int }
	var got int
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").Activity(func(_ context.Context, p any) error {
			got = p.(*payload).Value
			return nil
		}))

	wf.Signal(ctx, "A", "go", &payload{Value: 42}) //nolint:errcheck
	if got != 42 {
		t.Errorf("payload value = %d, want 42", got)
	}
}

// ─────────────────────────────────────────────────────────────
// OnEnter / OnExit Activities
// ─────────────────────────────────────────────────────────────

func TestOnEnter_CalledAfterTransition(t *testing.T) {
	var n int32
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		OnEnter("B", activityCounter(&n)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if n != 1 {
		t.Errorf("OnEnter called %d times, want 1", n)
	}
}

func TestExecutionOrder_ExitActivityEnter(t *testing.T) {
	var order []string
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		Activity(func(_ context.Context, _ any) error {
			order = append(order, "activity")
			return nil
		}).
		OnExit("A", func(_ context.Context, _ any) error {
			order = append(order, "exit")
			return nil
		}).
		OnEnter("B", func(_ context.Context, _ any) error {
			order = append(order, "enter")
			return nil
		}))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	want := []string{"exit", "activity", "enter"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Errorf("execution order = %v, want %v", order, want)
	}
}

func TestOnEnter_NotCalledOnActivityError(t *testing.T) {
	var n int32
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		Activity(func(_ context.Context, _ any) error { return errors.New("oops") }).
		OnEnter("B", activityCounter(&n)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if n != 0 {
		t.Errorf("OnEnter should not run when activity fails, called %d times", n)
	}
}

func TestOnExit_ErrorAbortsTransition(t *testing.T) {
	sentinel := errors.New("exit failed")
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		OnExit("A", func(_ context.Context, _ any) error { return sentinel }))

	state, err := wf.Signal(ctx, "A", "go", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
	if state != "A" {
		t.Errorf("state = %q after OnExit error, want A", state)
	}
}

func TestMultipleOnEnterGroups_Sequential(t *testing.T) {
	var order []int
	hook := func(i int) workflow.Activity {
		return func(_ context.Context, _ any) error {
			order = append(order, i)
			return nil
		}
	}
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		OnEnter("B", hook(1), hook(2)).
		OnEnter("B", hook(3)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if fmt.Sprint(order) != fmt.Sprint([]int{1, 2, 3}) {
		t.Errorf("order = %v, want [1 2 3]", order)
	}
}

// ─────────────────────────────────────────────────────────────
// Concurrent OnEnter / OnExit Activities
// ─────────────────────────────────────────────────────────────

func TestOnEnterParallel_AllActivitiesRun(t *testing.T) {
	var n int32
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		OnEnterParallel("B", activityCounter(&n), activityCounter(&n), activityCounter(&n)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if n != 3 {
		t.Errorf("parallel OnEnter activities ran %d times, want 3", n)
	}
}

func TestOnExitParallel_AllActivitiesRun(t *testing.T) {
	var n int32
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		OnExitParallel("A", activityCounter(&n), activityCounter(&n)))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if n != 2 {
		t.Errorf("parallel OnExit activities ran %d times, want 2", n)
	}
}

func TestOnEnterParallel_ErrorReturned(t *testing.T) {
	sentinel := errors.New("parallel activity failed")
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		OnEnterParallel("B",
			func(_ context.Context, _ any) error { return sentinel },
			func(_ context.Context, _ any) error { return nil },
		))

	_, err := wf.Signal(ctx, "A", "go", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────
// Conditional routing (If / ElseIf / Else)
// ─────────────────────────────────────────────────────────────

func TestIfElse_FirstConditionMatches(t *testing.T) {
	isHigh := func(_ context.Context, p any) bool { return p.(int) > 100 }
	isMed := func(_ context.Context, p any) bool { return p.(int) > 50 }

	wf := mustBuild(t, workflow.Define().
		From("PENDING").On("process").
		If(isHigh).To("HIGH").
		ElseIf(isMed).To("MEDIUM").
		Else("LOW"))

	got, err := wf.Signal(ctx, "PENDING", "process", 200)
	if err != nil || got != "HIGH" {
		t.Errorf("Signal(200) = (%q, %v), want (HIGH, nil)", got, err)
	}
}

func TestIfElse_SecondConditionMatches(t *testing.T) {
	isHigh := func(_ context.Context, p any) bool { return p.(int) > 100 }
	isMed := func(_ context.Context, p any) bool { return p.(int) > 50 }

	wf := mustBuild(t, workflow.Define().
		From("PENDING").On("process").
		If(isHigh).To("HIGH").
		ElseIf(isMed).To("MEDIUM").
		Else("LOW"))

	got, _ := wf.Signal(ctx, "PENDING", "process", 75)
	if got != "MEDIUM" {
		t.Errorf("state = %q, want MEDIUM", got)
	}
}

func TestIfElse_ElseFallback(t *testing.T) {
	never := func(_ context.Context, _ any) bool { return false }

	wf := mustBuild(t, workflow.Define().
		From("PENDING").On("process").
		If(never).To("HIGH").
		Else("FALLBACK"))

	got, _ := wf.Signal(ctx, "PENDING", "process", nil)
	if got != "FALLBACK" {
		t.Errorf("state = %q, want FALLBACK", got)
	}
}

func TestIfElse_NoMatchNoElse_ReturnsError(t *testing.T) {
	never := func(_ context.Context, _ any) bool { return false }

	wf := mustBuild(t, workflow.Define().
		From("PENDING").On("process").
		If(never).To("HIGH"))

	state, err := wf.Signal(ctx, "PENDING", "process", nil)
	if !errors.Is(err, workflow.ErrNoConditionMatched) {
		t.Fatalf("want ErrNoConditionMatched, got %v", err)
	}
	if state != "PENDING" {
		t.Errorf("state = %q after error, want PENDING", state)
	}
}

// ─────────────────────────────────────────────────────────────
// Switch routing
// ─────────────────────────────────────────────────────────────

func TestSwitch_MatchingCase(t *testing.T) {
	method := func(_ context.Context, p any) any { return p.(string) }

	wf := mustBuild(t, workflow.Define().
		From("PENDING").On("pay").
		Switch(method).
		Case("card", "CARD").
		Case("paypal", "PAYPAL").
		Default("UNSUPPORTED"))

	for _, tc := range []struct{ input, want string }{
		{"card", "CARD"},
		{"paypal", "PAYPAL"},
		{"unknown", "UNSUPPORTED"},
	} {
		got, err := wf.Signal(ctx, "PENDING", "pay", tc.input)
		if err != nil || got != tc.want {
			t.Errorf("input=%q: got (%q,%v), want (%q,nil)", tc.input, got, err, tc.want)
		}
	}
}

func TestSwitch_NoMatchNoDefault_ReturnsError(t *testing.T) {
	expr := func(_ context.Context, p any) any { return p }
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").
		Switch(expr).
		Case("x", "X"))

	state, err := wf.Signal(ctx, "A", "go", "unknown")
	if !errors.Is(err, workflow.ErrNoConditionMatched) {
		t.Fatalf("want ErrNoConditionMatched, got %v", err)
	}
	if state != "A" {
		t.Errorf("state should be unchanged, got %q", state)
	}
}

// ─────────────────────────────────────────────────────────────
// Execution (stateful wrapper)
// ─────────────────────────────────────────────────────────────

func TestExecution_TracksState(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		From("B").On("back").To("A"))

	exec := wf.NewExecution("A")
	if exec.CurrentState() != "A" {
		t.Fatalf("initial state = %q, want A", exec.CurrentState())
	}

	if err := exec.Signal(ctx, "go", nil); err != nil {
		t.Fatalf("Signal(go): %v", err)
	}
	if exec.CurrentState() != "B" {
		t.Errorf("after go: state = %q, want B", exec.CurrentState())
	}

	if err := exec.Signal(ctx, "back", nil); err != nil {
		t.Fatalf("Signal(back): %v", err)
	}
	if exec.CurrentState() != "A" {
		t.Errorf("after back: state = %q, want A", exec.CurrentState())
	}
}

func TestExecution_ErrorPreservesState(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		Activity(func(_ context.Context, _ any) error { return errors.New("oops") }))

	exec := wf.NewExecution("A")
	_ = exec.Signal(ctx, "go", nil)
	if exec.CurrentState() != "A" {
		t.Errorf("state = %q after error, want A", exec.CurrentState())
	}
}

func TestExecution_CanReceive(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B"))

	exec := wf.NewExecution("A")
	if !exec.CanReceive("go") {
		t.Error("CanReceive(go) = false, want true")
	}
	if exec.CanReceive("fly") {
		t.Error("CanReceive(fly) = true, want false")
	}
}

func TestExecution_SetState(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B"))

	exec := wf.NewExecution("A")
	exec.Signal(ctx, "go", nil) //nolint:errcheck
	exec.SetState("A")
	if exec.CurrentState() != "A" {
		t.Errorf("after SetState: state = %q, want A", exec.CurrentState())
	}
}

func TestExecution_ConcurrentSignals(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		From("B").On("go").To("A"))

	exec := wf.NewExecution("A")
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			exec.Signal(ctx, "go", nil) //nolint:errcheck
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	// No race → concurrent safety confirmed.
}

// ─────────────────────────────────────────────────────────────
// Workflow helpers
// ─────────────────────────────────────────────────────────────

func TestWorkflow_AvailableSignals(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		From("A").On("fly").To("C"))

	sigs := wf.AvailableSignals("A")
	if len(sigs) != 2 || sigs[0] != "fly" || sigs[1] != "go" {
		t.Errorf("AvailableSignals = %v, want [fly go]", sigs)
	}
	if len(wf.AvailableSignals("UNKNOWN")) != 0 {
		t.Error("expected empty for unknown state")
	}
}

func TestWorkflow_States(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		From("B").On("go").To("C"))

	if len(wf.States()) != 2 {
		t.Errorf("States() = %v, want 2 entries", wf.States())
	}
}

func TestWorkflow_Visualize_NotEmpty(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B"))

	if v := wf.Visualize(); v == "" {
		t.Error("Visualize() returned empty string")
	}
}

// ─────────────────────────────────────────────────────────────
// Logger
// ─────────────────────────────────────────────────────────────

type captureLogger struct{ from, signal, to string }

func (l *captureLogger) LogTransition(from, signal, to string) {
	l.from, l.signal, l.to = from, signal, to
}

func TestWithLogger_RecordsTransition(t *testing.T) {
	l := &captureLogger{}
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		WithLogger(l))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck
	if l.from != "A" || l.signal != "go" || l.to != "B" {
		t.Errorf("logger got (%q,%q,%q), want (A,go,B)", l.from, l.signal, l.to)
	}
}

func TestNoopLogger(t *testing.T) {
	wf := mustBuild(t, workflow.Define().
		From("A").On("go").To("B").
		WithLogger(workflow.NoopLogger{}))

	wf.Signal(ctx, "A", "go", nil) //nolint:errcheck // must not panic
}

// ─────────────────────────────────────────────────────────────
// MustBuild
// ─────────────────────────────────────────────────────────────

func TestMustBuild_PanicsOnDuplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustBuild on duplicate transition")
		}
	}()
	workflow.Define().
		From("A").On("go").To("B").
		From("A").On("go").To("C").
		MustBuild()
}
