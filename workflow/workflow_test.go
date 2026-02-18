package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

type buildable[T any] interface {
	Build() (*workflow.Workflow[T], error)
}

func mustBuild[T any](t *testing.T, b buildable[T]) *workflow.Workflow[T] {
	t.Helper()
	wf, err := b.Build()
	if err != nil {
		t.Fatalf("Build() unexpected error: %v", err)
	}
	return wf
}

func mustBuildErr[T any](t *testing.T, b buildable[T]) error {
	t.Helper()
	_, err := b.Build()
	if err == nil {
		t.Fatal("Build() expected error, got nil")
	}
	return err
}

var bg = context.Background()

// recordAny is a simple payload type used across most tests.
type recordAny = any

// act returns an Activity[any] that appends tag to *calls (thread-safe).
func act(calls *[]string, mu *sync.Mutex, tag string) workflow.Activity[any] {
	return func(_ context.Context, _ any) error {
		mu.Lock()
		*calls = append(*calls, tag)
		mu.Unlock()
		return nil
	}
}

// failAct returns an Activity[any] that returns errTest.
var errTest = errors.New("test-error")

func failAct(_ context.Context, _ any) error { return errTest }

// ─────────────────────────────────────────────────────────────────────────────
// 1. Simple transitions
// ─────────────────────────────────────────────────────────────────────────────

func TestSimpleTransition(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	got, err := wf.Signal(bg, "A", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "B" {
		t.Fatalf("state: want B, got %s", got)
	}
}

func TestMultipleFromStates(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A", "B").On("reset").To("INIT"))

	for _, from := range []string{"A", "B"} {
		got, err := wf.Signal(bg, from, "reset", nil)
		if err != nil {
			t.Fatalf("state=%s: unexpected error: %v", from, err)
		}
		if got != "INIT" {
			t.Fatalf("state=%s: want INIT, got %s", from, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. Error cases
// ─────────────────────────────────────────────────────────────────────────────

func TestUnknownSignal(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))

	_, err := wf.Signal(bg, "A", "nope", nil)
	if !errors.Is(err, workflow.ErrUnknownSignal) {
		t.Fatalf("want ErrUnknownSignal, got %v", err)
	}
}

func TestUnknownState(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))

	_, err := wf.Signal(bg, "UNKNOWN", "go", nil)
	if !errors.Is(err, workflow.ErrUnknownSignal) {
		t.Fatalf("want ErrUnknownSignal, got %v", err)
	}
}

func TestDuplicateTransitionError(t *testing.T) {
	mustBuildErr[any](t, workflow.Define[any]().
		From("A").On("go").To("B").
		From("A").On("go").To("C"))
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. Transition Activities
// ─────────────────────────────────────────────────────────────────────────────

func TestTransitionActivities_RunInOrder(t *testing.T) {
	var (
		calls []string
		mu    sync.Mutex
	)
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(act(&calls, &mu, "first"), act(&calls, &mu, "second")))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "first,second" {
		t.Fatalf("want first,second; got %s", got)
	}
}

func TestTransitionActivity_Failure_StateUnchanged(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(failAct))

	got, err := wf.Signal(bg, "A", "go", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got != "A" {
		t.Fatalf("state should stay A, got %s", got)
	}
}

func TestTransitionActivity_PayloadPassthrough(t *testing.T) {
	type order struct{ Amount int }
	var received *order

	wf := mustBuild(t, workflow.Define[*order]().
		From("PENDING").On("approve").To("APPROVED").
		Activity(func(_ context.Context, o *order) error {
			received = o
			return nil
		}))

	o := &order{Amount: 42}
	if _, err := wf.Signal(bg, "PENDING", "approve", o); err != nil {
		t.Fatal(err)
	}
	if received != o || received.Amount != 42 {
		t.Fatal("payload not passed through correctly")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. State hooks — sequential
// ─────────────────────────────────────────────────────────────────────────────

func TestStateHooks_Sequential(t *testing.T) {
	var (
		calls []string
		mu    sync.Mutex
	)
	wf := mustBuild(t, workflow.Define[any]().
		OnExit("A", act(&calls, &mu, "exitA")).
		OnEnter("B", act(&calls, &mu, "enterB")).
		From("A").On("go").To("B").
		Activity(act(&calls, &mu, "transition")))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}
	want := "exitA,transition,enterB"
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("want %s; got %s", want, got)
	}
}

func TestExitHook_Failure_DoesNotTransition(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		OnExit("A", failAct).
		From("A").On("go").To("B"))

	got, err := wf.Signal(bg, "A", "go", nil)
	if err == nil {
		t.Fatal("expected error from OnExit hook")
	}
	if got != "A" {
		t.Fatalf("state should stay A, got %s", got)
	}
}

func TestEnterHook_Failure_StateIsUpdated(t *testing.T) {
	// OnEnter failure: destination state is already set, returns (dst, err).
	wf := mustBuild(t, workflow.Define[any]().
		OnEnter("B", failAct).
		From("A").On("go").To("B"))

	got, err := wf.Signal(bg, "A", "go", nil)
	if err == nil {
		t.Fatal("expected error from OnEnter hook")
	}
	if got != "B" {
		t.Fatalf("state should be B (already transitioned), got %s", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. State hooks — concurrent
// ─────────────────────────────────────────────────────────────────────────────

func TestStateHooks_Concurrent(t *testing.T) {
	var counter atomic.Int32
	var wg sync.WaitGroup

	hookFn := func(_ context.Context, _ any) error {
		wg.Done()
		time.Sleep(20 * time.Millisecond) // simulate real work
		counter.Add(1)
		return nil
	}

	wg.Add(3)
	wf := mustBuild(t, workflow.Define[any]().
		OnEnterConcurrent("B", hookFn, hookFn, hookFn).
		From("A").On("go").To("B"))

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("concurrent hooks did not all start within 200ms")
	}

	if counter.Load() != 3 {
		t.Fatalf("want 3 hook completions, got %d", counter.Load())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. Conditional routing — if/else
// ─────────────────────────────────────────────────────────────────────────────

func isApproved(_ context.Context, payload any) bool  { return payload.(bool) }
func isRejected(_ context.Context, payload any) bool  { return !payload.(bool) }

func TestConditional_IfBranch(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("PENDING").On("decide").
		If(isApproved, "APPROVED").
		Else("REJECTED"))

	got, err := wf.Signal(bg, "PENDING", "decide", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "APPROVED" {
		t.Fatalf("want APPROVED, got %s", got)
	}
}

func TestConditional_ElseBranch(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("PENDING").On("decide").
		If(isApproved, "APPROVED").
		Else("REJECTED"))

	got, err := wf.Signal(bg, "PENDING", "decide", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "REJECTED" {
		t.Fatalf("want REJECTED, got %s", got)
	}
}

func TestConditional_NoMatchNoElse_Error(t *testing.T) {
	// Both conditions check for specific string values; an unrecognised string matches neither.
	isHigh := func(_ context.Context, p any) bool { return p.(string) == "high" }
	isLow := func(_ context.Context, p any) bool { return p.(string) == "low" }

	wf := mustBuild(t, workflow.Define[any]().
		From("PENDING").On("decide").
		If(isHigh, "HIGH_STATE").
		ElseIf(isLow, "LOW_STATE"))

	_, err := wf.Signal(bg, "PENDING", "decide", "medium")
	if !errors.Is(err, workflow.ErrNoConditionMatched) {
		t.Fatalf("want ErrNoConditionMatched, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 7. Conditional routing — switch
// ─────────────────────────────────────────────────────────────────────────────

var orderStatusExpr workflow.SwitchExpr[any] = func(_ context.Context, payload any) any {
	return payload.(string)
}

func TestSwitch_MatchesCase(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("PENDING").On("route").
		Switch(orderStatusExpr).
		Case("premium", "VIP_PROCESSING").
		Case("standard", "STANDARD_PROCESSING").
		Default("GENERAL_PROCESSING"))

	got, err := wf.Signal(bg, "PENDING", "route", "premium")
	if err != nil {
		t.Fatal(err)
	}
	if got != "VIP_PROCESSING" {
		t.Fatalf("want VIP_PROCESSING, got %s", got)
	}
}

func TestSwitch_DefaultCase(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("PENDING").On("route").
		Switch(orderStatusExpr).
		Case("premium", "VIP_PROCESSING").
		Default("GENERAL_PROCESSING"))

	got, err := wf.Signal(bg, "PENDING", "route", "unknown")
	if err != nil {
		t.Fatal(err)
	}
	if got != "GENERAL_PROCESSING" {
		t.Fatalf("want GENERAL_PROCESSING, got %s", got)
	}
}

func TestSwitch_NoDefault_Error(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("PENDING").On("route").
		Switch(orderStatusExpr).
		Case("premium", "VIP_PROCESSING"))

	_, err := wf.Signal(bg, "PENDING", "route", "unknown")
	if !errors.Is(err, workflow.ErrNoConditionMatched) {
		t.Fatalf("want ErrNoConditionMatched, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. Execution lifecycle
// ─────────────────────────────────────────────────────────────────────────────

func TestExecution_Signal_UpdatesState(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		From("B").On("go").To("C"))

	exec := wf.NewExecution(bg, "A")

	if err := exec.Signal(bg, "go", nil); err != nil {
		t.Fatal(err)
	}
	if got := exec.CurrentState(); got != "B" {
		t.Fatalf("want B, got %s", got)
	}

	if err := exec.Signal(bg, "go", nil); err != nil {
		t.Fatal(err)
	}
	if got := exec.CurrentState(); got != "C" {
		t.Fatalf("want C, got %s", got)
	}
}

func TestExecution_CanReceive(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	exec := wf.NewExecution(bg, "A")
	if !exec.CanReceive("go") {
		t.Fatal("expected CanReceive(go)=true in state A")
	}
	if exec.CanReceive("nope") {
		t.Fatal("expected CanReceive(nope)=false in state A")
	}
}

func TestExecution_SetState(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	exec := wf.NewExecution(bg, "A")
	exec.SetState("B")
	if got := exec.CurrentState(); got != "B" {
		t.Fatalf("want B, got %s", got)
	}
}

func TestExecution_AvailableSignals(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("x").To("B").
		From("A").On("y").To("C"))

	exec := wf.NewExecution(bg, "A")
	sigs := exec.AvailableSignals()
	if len(sigs) != 2 || sigs[0] != "x" || sigs[1] != "y" {
		t.Fatalf("unexpected signals: %v", sigs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 9. Event History
// ─────────────────────────────────────────────────────────────────────────────

func TestHistory_RecordsSuccessfulTransitions(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		From("B").On("go").To("C"))

	exec := wf.NewExecution(bg, "A")
	_ = exec.Signal(bg, "go", nil)
	_ = exec.Signal(bg, "go", nil)

	h := exec.History()
	if len(h) != 2 {
		t.Fatalf("want 2 history entries, got %d", len(h))
	}
	if h[0].FromState != "A" || h[0].ToState != "B" || h[0].Signal != "go" || h[0].Err != nil {
		t.Fatalf("unexpected first entry: %+v", h[0])
	}
	if h[1].FromState != "B" || h[1].ToState != "C" || h[1].Err != nil {
		t.Fatalf("unexpected second entry: %+v", h[1])
	}
}

func TestHistory_RecordsErrors(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(failAct))

	exec := wf.NewExecution(bg, "A")
	_ = exec.Signal(bg, "go", nil)

	h := exec.History()
	if len(h) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(h))
	}
	if h[0].Err == nil {
		t.Fatal("expected error in history entry")
	}
	if h[0].FromState != "A" {
		t.Fatalf("want FromState=A, got %s", h[0].FromState)
	}
}

func TestHistory_DurationRecorded(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(func(_ context.Context, _ any) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		}))

	exec := wf.NewExecution(bg, "A")
	_ = exec.Signal(bg, "go", nil)

	h := exec.History()
	if h[0].Duration < 10*time.Millisecond {
		t.Fatalf("expected duration ≥ 10ms, got %v", h[0].Duration)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 10. Execution lifecycle hooks
// ─────────────────────────────────────────────────────────────────────────────

func TestExecutionHooks_OnTransition(t *testing.T) {
	var (
		mu    sync.Mutex
		froms []string
		tos   []string
	)
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	hooks := workflow.ExecutionHooks[any]{
		OnTransition: func(_ context.Context, from, to, signal string, _ any) {
			mu.Lock()
			froms = append(froms, from)
			tos = append(tos, to)
			mu.Unlock()
		},
	}
	exec := wf.NewExecution(bg, "A", workflow.WithHooks(hooks))
	_ = exec.Signal(bg, "go", nil)

	mu.Lock()
	defer mu.Unlock()
	if len(froms) != 1 || froms[0] != "A" || tos[0] != "B" {
		t.Fatalf("unexpected hook calls: from=%v to=%v", froms, tos)
	}
}

func TestExecutionHooks_OnError(t *testing.T) {
	var (
		mu         sync.Mutex
		errorState string
	)
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").Activity(failAct))

	hooks := workflow.ExecutionHooks[any]{
		OnError: func(_ context.Context, state, _ string, _ error, _ any) {
			mu.Lock()
			errorState = state
			mu.Unlock()
		},
	}
	exec := wf.NewExecution(bg, "A", workflow.WithHooks(hooks))
	_ = exec.Signal(bg, "go", nil)

	mu.Lock()
	defer mu.Unlock()
	if errorState != "A" {
		t.Fatalf("want errorState=A, got %s", errorState)
	}
}

func TestExecutionHooks_NoTransitionHookOnError(t *testing.T) {
	var transitionCalled bool
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").Activity(failAct))

	hooks := workflow.ExecutionHooks[any]{
		OnTransition: func(_ context.Context, _, _, _ string, _ any) {
			transitionCalled = true
		},
	}
	exec := wf.NewExecution(bg, "A", workflow.WithHooks(hooks))
	_ = exec.Signal(bg, "go", nil)

	if transitionCalled {
		t.Fatal("OnTransition should not fire when Signal fails")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 11. Cancellation
// ─────────────────────────────────────────────────────────────────────────────

func TestExecution_Cancel_PreventsSignals(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))

	exec := wf.NewExecution(bg, "A")
	exec.Cancel()

	err := exec.Signal(bg, "go", nil)
	if !errors.Is(err, workflow.ErrExecutionCancelled) {
		t.Fatalf("want ErrExecutionCancelled, got %v", err)
	}
}

func TestExecution_Cancel_InterruptsActivity(t *testing.T) {
	started := make(chan struct{})

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(func(ctx context.Context, _ any) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}))

	exec := wf.NewExecution(bg, "A")

	errCh := make(chan error, 1)
	go func() {
		errCh <- exec.Signal(bg, "go", nil)
	}()

	// Wait for the activity to start, then cancel.
	<-started
	exec.Cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("Signal did not return after Cancel")
	}
}

func TestExecution_Done_ClosedAfterCancel(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))
	exec := wf.NewExecution(bg, "A")
	exec.Cancel()

	select {
	case <-exec.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done() channel not closed after Cancel")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 12. Await
// ─────────────────────────────────────────────────────────────────────────────

func TestAwait_UnblocksOnStateChange(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	exec := wf.NewExecution(bg, "A")

	ctx, cancel := context.WithTimeout(bg, 2*time.Second)
	defer cancel()

	// Signal concurrently after a short delay.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = exec.Signal(bg, "go", nil)
	}()

	err := exec.Await(ctx, func(state string) bool { return state == "B" })
	if err != nil {
		t.Fatalf("Await returned error: %v", err)
	}
	if exec.CurrentState() != "B" {
		t.Fatal("state should be B after Await")
	}
}

func TestAwait_ReturnsOnContextCancel(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))
	exec := wf.NewExecution(bg, "A")

	ctx, cancel := context.WithCancel(bg)
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := exec.Await(ctx, func(_ string) bool { return false })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestAwait_ReturnsOnExecutionCancel(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))
	exec := wf.NewExecution(bg, "A")

	go func() {
		time.Sleep(20 * time.Millisecond)
		exec.Cancel()
	}()

	err := exec.Await(bg, func(_ string) bool { return false })
	if !errors.Is(err, workflow.ErrExecutionCancelled) {
		t.Fatalf("want ErrExecutionCancelled, got %v", err)
	}
}

func TestAwait_ImmediatelyTrueCondition(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().From("A").On("go").To("B"))
	exec := wf.NewExecution(bg, "A")

	err := exec.Await(bg, func(_ string) bool { return true })
	if err != nil {
		t.Fatalf("expected nil error when condition already true, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 13. Retry
// ─────────────────────────────────────────────────────────────────────────────

func TestRetry_SucceedsOnSecondAttempt(t *testing.T) {
	var attempts int
	flaky := func(_ context.Context, _ any) error {
		attempts++
		if attempts < 2 {
			return errTest
		}
		return nil
	}

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Retry(flaky, workflow.RetryPolicy{
			MaxAttempts:     3,
			InitialInterval: time.Millisecond,
		})))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("want 2 attempts, got %d", attempts)
	}
}

func TestRetry_ExhaustsMaxAttempts(t *testing.T) {
	var attempts int
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Retry(func(_ context.Context, _ any) error {
			attempts++
			return errTest
		}, workflow.RetryPolicy{
			MaxAttempts:     3,
			InitialInterval: time.Millisecond,
		})))

	_, err := wf.Signal(bg, "A", "go", nil)
	if !errors.Is(err, workflow.ErrRetryExhausted) {
		t.Fatalf("want ErrRetryExhausted, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("want 3 attempts, got %d", attempts)
	}
}

func TestRetry_SkipsNonRetryableError(t *testing.T) {
	var nonRetryable = errors.New("non-retryable")
	var attempts int

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Retry(func(_ context.Context, _ any) error {
			attempts++
			return nonRetryable
		}, workflow.RetryPolicy{
			MaxAttempts:        5,
			InitialInterval:    time.Millisecond,
			NonRetryableErrors: []error{nonRetryable},
		})))

	_, err := wf.Signal(bg, "A", "go", nil)
	if !errors.Is(err, nonRetryable) {
		t.Fatalf("want nonRetryable error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("want 1 attempt (no retry), got %d", attempts)
	}
}

func TestRetry_UnlimitedUntilContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(bg, 50*time.Millisecond)
	defer cancel()

	var attempts int
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Retry(func(_ context.Context, _ any) error {
			attempts++
			return errTest
		}, workflow.RetryPolicy{
			MaxAttempts:     0, // unlimited
			InitialInterval: time.Millisecond,
		})))

	_, err := wf.Signal(ctx, "A", "go", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts < 2 {
		t.Fatalf("expected multiple attempts, got %d", attempts)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	var times []time.Time
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Retry(func(_ context.Context, _ any) error {
			times = append(times, time.Now())
			return errTest
		}, workflow.RetryPolicy{
			MaxAttempts:        3,
			InitialInterval:    20 * time.Millisecond,
			BackoffCoefficient: 2.0,
		})))

	wf.Signal(bg, "A", "go", nil) //nolint:errcheck

	if len(times) != 3 {
		t.Fatalf("want 3 attempt timestamps, got %d", len(times))
	}
	// First gap ≈ 20ms, second ≈ 40ms. Allow generous margin for CI.
	gap1 := times[1].Sub(times[0])
	gap2 := times[2].Sub(times[1])
	if gap2 < gap1 {
		t.Fatalf("backoff should grow: gap1=%v gap2=%v", gap1, gap2)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 14. Timeout
// ─────────────────────────────────────────────────────────────────────────────

func TestTimeout_CancelsActivity(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Timeout(func(ctx context.Context, _ any) error {
			<-ctx.Done()
			return ctx.Err()
		}, 30*time.Millisecond)))

	start := time.Now()
	_, err := wf.Signal(bg, "A", "go", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestTimeout_SuccessWithinDeadline(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.Timeout(func(_ context.Context, _ any) error {
			return nil // finishes immediately
		}, time.Second)))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 15. Middleware
// ─────────────────────────────────────────────────────────────────────────────

func TestWithMiddleware_WrapsActivity(t *testing.T) {
	var order []string
	var mu sync.Mutex

	append := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	outer := func(next workflow.Activity[any]) workflow.Activity[any] {
		return func(ctx context.Context, p any) error {
			append("outer-before")
			err := next(ctx, p)
			append("outer-after")
			return err
		}
	}
	inner := func(next workflow.Activity[any]) workflow.Activity[any] {
		return func(ctx context.Context, p any) error {
			append("inner-before")
			err := next(ctx, p)
			append("inner-after")
			return err
		}
	}
	core := func(_ context.Context, _ any) error {
		append("core")
		return nil
	}

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(workflow.WithMiddleware(core, outer, inner)))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := "outer-before,inner-before,core,inner-after,outer-after"
	got := strings.Join(order, ",")
	if got != want {
		t.Fatalf("middleware order: want %s, got %s", want, got)
	}
}

func TestBuilderMiddleware_AppliedToAllActivities(t *testing.T) {
	var callCount atomic.Int32

	mw := func(next workflow.Activity[any]) workflow.Activity[any] {
		return func(ctx context.Context, p any) error {
			callCount.Add(1)
			return next(ctx, p)
		}
	}

	noop := func(_ context.Context, _ any) error { return nil }

	wf := mustBuild(t, workflow.Define[any]().
		WithMiddleware(mw).
		OnEnter("B", noop).          // hook — should also be wrapped
		From("A").On("go").To("B").
		Activity(noop, noop))        // two transition activities

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}

	// 2 transition activities + 1 OnEnter hook = 3 middleware calls.
	if callCount.Load() != 3 {
		t.Fatalf("want 3 middleware calls, got %d", callCount.Load())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 16. Saga compensation
// ─────────────────────────────────────────────────────────────────────────────

func TestSaga_NoFailure_NoCompensation(t *testing.T) {
	var compensated []string
	var mu sync.Mutex

	comp := func(tag string) workflow.Activity[any] {
		return func(_ context.Context, _ any) error {
			mu.Lock()
			compensated = append(compensated, tag)
			mu.Unlock()
			return nil
		}
	}
	noop := func(_ context.Context, _ any) error { return nil }

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Saga(noop, comp("comp1")).
		Saga(noop, comp("comp2")))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(compensated) != 0 {
		t.Fatalf("expected no compensations, got %v", compensated)
	}
}

func TestSaga_SecondStepFails_FirstCompensated(t *testing.T) {
	var (
		calls       []string
		mu          sync.Mutex
	)

	record := func(tag string) workflow.Activity[any] {
		return func(_ context.Context, _ any) error {
			mu.Lock()
			calls = append(calls, tag)
			mu.Unlock()
			return nil
		}
	}
	fail := func(_ context.Context, _ any) error { return errTest }

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Saga(record("step1"), record("comp1")).
		Saga(fail, record("comp2")))

	_, err := wf.Signal(bg, "A", "go", nil)
	if err == nil {
		t.Fatal("expected error from failed Saga step")
	}

	mu.Lock()
	defer mu.Unlock()
	// step1 ran, step2 (fail) ran, then comp1 compensated. comp2 not called (step2 failed).
	if !contains(calls, "step1") {
		t.Fatalf("step1 should have run; calls=%v", calls)
	}
	if !contains(calls, "comp1") {
		t.Fatalf("comp1 should have run as compensation; calls=%v", calls)
	}
	if contains(calls, "comp2") {
		t.Fatalf("comp2 should NOT run (step2 never succeeded); calls=%v", calls)
	}
}

func TestSaga_ThirdStepFails_CompensatesInReverse(t *testing.T) {
	var (
		order []string
		mu    sync.Mutex
	)

	recordAct := func(tag string) workflow.Activity[any] {
		return func(_ context.Context, _ any) error {
			mu.Lock()
			order = append(order, "act:"+tag)
			mu.Unlock()
			return nil
		}
	}
	recordComp := func(tag string) workflow.Activity[any] {
		return func(_ context.Context, _ any) error {
			mu.Lock()
			order = append(order, "comp:"+tag)
			mu.Unlock()
			return nil
		}
	}
	fail := func(_ context.Context, _ any) error { return errTest }

	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Saga(recordAct("1"), recordComp("1")).
		Saga(recordAct("2"), recordComp("2")).
		Saga(fail, recordComp("3")))

	wf.Signal(bg, "A", "go", nil) //nolint:errcheck

	mu.Lock()
	defer mu.Unlock()
	// Expected order: act:1, act:2, (fail), comp:2, comp:1
	expected := []string{"act:1", "act:2", "comp:2", "comp:1"}
	if strings.Join(order, ",") != strings.Join(expected, ",") {
		t.Fatalf("saga order mismatch:\n  want: %v\n  got:  %v", expected, order)
	}
}

func TestSaga_MixedWithActivity(t *testing.T) {
	var calls []string
	var mu sync.Mutex

	record := func(tag string) workflow.Activity[any] {
		return func(_ context.Context, _ any) error {
			mu.Lock()
			calls = append(calls, tag)
			mu.Unlock()
			return nil
		}
	}

	// Plain Activity followed by Saga step; only the Saga step has compensation.
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		Activity(record("plain")).
		Saga(record("saga"), record("comp")))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(calls, ",") != "plain,saga" {
		t.Fatalf("unexpected calls: %v", calls)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 17. Logger
// ─────────────────────────────────────────────────────────────────────────────

type captureLogger struct {
	mu   sync.Mutex
	logs []string
}

func (l *captureLogger) LogTransition(from, signal, to string) {
	l.mu.Lock()
	l.logs = append(l.logs, fmt.Sprintf("%s--%s-->%s", from, signal, to))
	l.mu.Unlock()
}

func TestLogger_LogsTransition(t *testing.T) {
	logger := &captureLogger{}
	wf := mustBuild(t, workflow.Define[any]().
		WithLogger(logger).
		From("A").On("go").To("B"))

	if _, err := wf.Signal(bg, "A", "go", nil); err != nil {
		t.Fatal(err)
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.logs) != 1 || logger.logs[0] != "A--go-->B" {
		t.Fatalf("unexpected logs: %v", logger.logs)
	}
}

func TestLogger_NotCalledOnError(t *testing.T) {
	logger := &captureLogger{}
	wf := mustBuild(t, workflow.Define[any]().
		WithLogger(logger).
		From("A").On("go").To("B").Activity(failAct))

	wf.Signal(bg, "A", "go", nil) //nolint:errcheck

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.logs) != 0 {
		t.Fatalf("logger should not log on activity failure: %v", logger.logs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 18. Workflow metadata
// ─────────────────────────────────────────────────────────────────────────────

func TestAvailableSignals(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("x").To("B").
		From("A").On("y").To("C"))

	sigs := wf.AvailableSignals("A")
	if len(sigs) != 2 || sigs[0] != "x" || sigs[1] != "y" {
		t.Fatalf("unexpected signals: %v", sigs)
	}
	if len(wf.AvailableSignals("B")) != 0 {
		t.Fatal("B should have no outgoing signals")
	}
}

func TestStates(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B").
		From("B").On("go").To("C"))

	states := wf.States()
	// Only states with outgoing transitions are listed.
	if len(states) != 2 || states[0] != "A" || states[1] != "B" {
		t.Fatalf("unexpected states: %v", states)
	}
}

func TestVisualize(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	v := wf.Visualize()
	if !strings.Contains(v, "A") || !strings.Contains(v, "go") || !strings.Contains(v, "B") {
		t.Fatalf("Visualize output missing expected content: %s", v)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 19. Concurrency / race detector
// ─────────────────────────────────────────────────────────────────────────────

func TestExecution_ConcurrentSignalsSerialised(t *testing.T) {
	// Transitions A→B, B→A form a loop.
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("toggle").To("B").
		From("B").On("toggle").To("A"))

	exec := wf.NewExecution(bg, "A")

	const goroutines = 10
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = exec.Signal(bg, "toggle", nil)
		}()
	}
	wg.Wait()
	// State must be a valid state (A or B) — no panics or data races.
	s := exec.CurrentState()
	if s != "A" && s != "B" {
		t.Fatalf("unexpected state after concurrent signals: %s", s)
	}
}

func TestWorkflow_SharedAcrossGoroutines(t *testing.T) {
	wf := mustBuild(t, workflow.Define[any]().
		From("A").On("go").To("B"))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := wf.Signal(bg, "A", "go", nil)
			if err != nil || got != "B" {
				t.Errorf("unexpected result: state=%s err=%v", got, err)
			}
		}()
	}
	wg.Wait()
}

// ─────────────────────────────────────────────────────────────────────────────
// 20. Type-safe generics
// ─────────────────────────────────────────────────────────────────────────────

func TestGenerics_TypedPayload(t *testing.T) {
	type Payment struct {
		Amount   float64
		Currency string
	}

	var received *Payment
	wf := mustBuild(t, workflow.Define[*Payment]().
		From("PENDING").On("charge").To("CHARGED").
		Activity(func(_ context.Context, p *Payment) error {
			received = p
			return nil
		}))

	p := &Payment{Amount: 9.99, Currency: "USD"}
	if _, err := wf.Signal(bg, "PENDING", "charge", p); err != nil {
		t.Fatal(err)
	}
	if received == nil || received.Amount != 9.99 {
		t.Fatalf("typed payload not received correctly: %+v", received)
	}
}

func TestGenerics_ConditionTyped(t *testing.T) {
	type Claim struct{ Approved bool }

	wf := mustBuild(t, workflow.Define[*Claim]().
		From("REVIEW").On("decide").
		If(func(_ context.Context, c *Claim) bool { return c.Approved }, "APPROVED").
		Else("DENIED"))

	got, err := wf.Signal(bg, "REVIEW", "decide", &Claim{Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "APPROVED" {
		t.Fatalf("want APPROVED, got %s", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
