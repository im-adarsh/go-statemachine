// # Middleware Example
//
// Demonstrates all three ways to apply cross-cutting behaviour to Activities:
//
//   1. Workflow-wide middleware  — builder.WithMiddleware(mw...)
//      Applied at Build() time to every activity and state hook.
//      Use for observability (OpenTelemetry, Prometheus, structured logging).
//
//   2. Per-activity middleware  — workflow.WithMiddleware(fn, mw...)
//      Wraps a single activity before it is registered.
//      Use for targeted retry / circuit-breaker / custom logic.
//
//   3. Middleware composition   — first arg is outermost wrapper.
//      outer(inner(core)) — outer executes first and last.
//
// The example builds a payment workflow instrumented with:
//   - A structured logger (all activities)
//   - A latency recorder (all activities)
//   - A circuit breaker stub (only the external payment gateway call)
//   - A timeout (only the fraud check call)
//
// Run:
//
//	go run ./examples/middleware
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain
// ─────────────────────────────────────────────────────────────────────────────

type Payment struct {
	ID       string
	Amount   float64
	Currency string
	Method   string // "card" | "bank"
}

// ─────────────────────────────────────────────────────────────────────────────
// Core activities (pure business logic — no middleware concerns)
// ─────────────────────────────────────────────────────────────────────────────

func validatePayment(_ context.Context, p *Payment) error {
	if p.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	fmt.Printf("  [%s] validate: $%.2f %s via %s\n", p.ID, p.Amount, p.Currency, p.Method)
	return nil
}

func runFraudCheck(_ context.Context, p *Payment) error {
	time.Sleep(15 * time.Millisecond) // simulate external fraud service
	fmt.Printf("  [%s] fraud check: passed\n", p.ID)
	return nil
}

var gatewayCallCount atomic.Int32

func callPaymentGateway(_ context.Context, p *Payment) error {
	n := gatewayCallCount.Add(1)
	// Simulate intermittent gateway failure on first call.
	if n == 1 {
		fmt.Printf("  [%s] gateway: temporary error (call #%d)\n", p.ID, n)
		return errors.New("gateway timeout")
	}
	fmt.Printf("  [%s] gateway: charged $%.2f (call #%d)\n", p.ID, p.Amount, n)
	return nil
}

func sendReceipt(_ context.Context, p *Payment) error {
	fmt.Printf("  [%s] receipt: sent for $%.2f %s\n", p.ID, p.Amount, p.Currency)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Middleware implementations
// ─────────────────────────────────────────────────────────────────────────────

// callLog is the name registry for middleware logging (function name lookup).
// In production, use runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name().
var activityNames = map[any]string{}

// structuredLogger wraps every activity with a consistent log format.
func structuredLogger(name string) workflow.Middleware[*Payment] {
	return func(next workflow.Activity[*Payment]) workflow.Activity[*Payment] {
		return func(ctx context.Context, p *Payment) error {
			start := time.Now()
			fmt.Printf("  → activity.start  name=%-22s payment=%s\n", name, p.ID)
			err := next(ctx, p)
			elapsed := time.Since(start).Round(time.Microsecond)
			if err != nil {
				fmt.Printf("  ← activity.error  name=%-22s latency=%-10v error=%v\n", name, elapsed, err)
			} else {
				fmt.Printf("  ← activity.done   name=%-22s latency=%v\n", name, elapsed)
			}
			return err
		}
	}
}

// latencyRecorder measures and accumulates total activity latency.
var totalLatency atomic.Int64

func latencyRecorder(next workflow.Activity[*Payment]) workflow.Activity[*Payment] {
	return func(ctx context.Context, p *Payment) error {
		start := time.Now()
		err := next(ctx, p)
		totalLatency.Add(int64(time.Since(start)))
		return err
	}
}

// circuitBreaker is a simplified circuit breaker: if the activity fails more
// than maxFailures times, it opens the circuit and rejects all further calls.
func circuitBreaker(maxFailures int32) workflow.Middleware[*Payment] {
	var failures atomic.Int32
	return func(next workflow.Activity[*Payment]) workflow.Activity[*Payment] {
		return func(ctx context.Context, p *Payment) error {
			if failures.Load() >= maxFailures {
				return fmt.Errorf("circuit open (failures=%d)", failures.Load())
			}
			err := next(ctx, p)
			if err != nil {
				failures.Add(1)
				fmt.Printf("  ⚡ circuit-breaker: failure #%d/%d\n", failures.Load(), maxFailures)
			} else {
				failures.Store(0) // reset on success
			}
			return err
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Retry policy for the payment gateway
// ─────────────────────────────────────────────────────────────────────────────

var gatewayRetry = workflow.RetryPolicy{
	MaxAttempts:     3,
	InitialInterval: 10 * time.Millisecond,
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow definition
// ─────────────────────────────────────────────────────────────────────────────

// We compose per-activity middleware inline using workflow.WithMiddleware.
// Then apply workflow-wide middleware (latencyRecorder) via builder.WithMiddleware.

var paymentWF = workflow.Define[*Payment]().
	// Workflow-wide: latency recorder wraps ALL activities including state hooks.
	WithMiddleware(latencyRecorder).

	// Validation — plain activity (workflow-wide latencyRecorder applies).
	From("IDLE").On("start").To("VALIDATING").
	Activity(validatePayment).

	// Fraud check — timeout wraps the slow external call;
	// structuredLogger is applied per-activity for targeted logging.
	From("VALIDATING").On("check").To("FRAUD_CHECKED").
	Activity(
		workflow.WithMiddleware(
			workflow.Timeout(runFraudCheck, 500*time.Millisecond),
			structuredLogger("fraud-check"),
		),
	).

	// Payment gateway — retry + circuit breaker + structured logger.
	// Composition order: logger(circuitBreaker(retry(gateway)))
	// → logger is outermost, retry is innermost around the real call.
	From("FRAUD_CHECKED").On("charge").To("CHARGED").
	Activity(
		workflow.WithMiddleware(
			workflow.Retry(callPaymentGateway, gatewayRetry),
			circuitBreaker(5),
			structuredLogger("payment-gateway"),
		),
	).

	// Receipt — plain activity.
	From("CHARGED").On("receipt").To("COMPLETED").
	Activity(sendReceipt).

	MustBuild()

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	ctx := context.Background()

	fmt.Println("┌─ Payment Workflow graph ────────────────────────────────────")
	fmt.Print(paymentWF.Visualize())
	fmt.Println("└────────────────────────────────────────────────────────────")
	fmt.Println()

	p := &Payment{ID: "PAY-001", Amount: 149.99, Currency: "USD", Method: "card"}

	exec := paymentWF.NewExecution(ctx, "IDLE",
		workflow.WithHooks(workflow.ExecutionHooks[*Payment]{
			OnTransition: func(_ context.Context, from, to, sig string, p *Payment) {
				fmt.Printf("\n[%s] %s --%s--> %s\n", p.ID, from, sig, to)
			},
			OnError: func(_ context.Context, state, sig string, err error, p *Payment) {
				fmt.Printf("[%s] ✗ %s in %s: %v\n", p.ID, sig, state, err)
			},
		}),
	)

	fmt.Println("── Driving payment through all signals ──────────────────────")
	for _, sig := range []string{"start", "check", "charge", "receipt"} {
		fmt.Printf("\n── signal: %q ──\n", sig)
		if err := exec.Signal(ctx, sig, p); err != nil {
			fmt.Printf("Signal %q returned error: %v\n", sig, err)
		}
	}

	fmt.Println()
	fmt.Println("── Event history ─────────────────────────────────────────────")
	fmt.Printf("%-4s  %-12s  %-8s  %-14s  %-10s  %s\n",
		"#", "From", "Signal", "To", "Latency", "Status")
	fmt.Println(strings.Repeat("─", 60))
	for i, e := range exec.History() {
		status := "✓"
		if e.Err != nil {
			status = "✗"
		}
		fmt.Printf("%-4d  %-12s  %-8s  %-14s  %-10v  %s\n",
			i+1, e.FromState, e.Signal, e.ToState,
			e.Duration.Round(time.Millisecond), status)
	}

	fmt.Printf("\nTotal activity latency (all layers): %v\n",
		time.Duration(totalLatency.Load()).Round(time.Millisecond))
	fmt.Printf("Final state: %s\n", exec.CurrentState())
}
