// ShopFlow — E-commerce Order Processing
//
// A single, realistic application that walks through every feature of
// go-statemachine. Each scenario below isolates one concept while reusing the
// same Order domain, the same Workflow definition, and the same activities.
//
// # The State Machine
//
//	                  ┌─────────────┐
//	                  │   PENDING   │
//	                  └──────┬──────┘
//	                validate │  (fraud check)
//	             ┌───────────┴────────────┐
//	             ▼                        ▼
//	      ┌──────────┐           ┌──────────────┐
//	      │ REJECTED │           │ PAYMENT_HOLD │
//	      └──────────┘           └──────┬───────┘
//	                               fulfil│  (saga: charge + inventory + carrier)
//	                                     ▼
//	                             ┌────────────────┐
//	                             │   FULFILLING   │◄──┐
//	                             └───────┬────────┘   │ cancel (from any state)
//	                                ship │            │
//	                                     ▼            │
//	                             ┌────────────────┐   │
//	                             │    SHIPPED     │───┘
//	                             └───────┬────────┘
//	                             deliver │
//	                                     ▼
//	                             ┌────────────────┐
//	                             │   DELIVERED    │
//	                             └────────────────┘
//
// # Scenarios
//
//   1. Happy Path       — basic transitions, OnEnter hooks, history, Await
//   2. Fraud Rejection  — If/Else conditional routing
//   3. Shipping Tiers   — Switch-case conditional routing
//   4. Saga Rollback    — carrier fails, card refunded + inventory released
//   5. Retry & Timeout  — flaky inventory + slow carrier bounded by deadline
//   6. Middleware       — per-activity + workflow-wide observability
//   7. Cancellation     — exec.Cancel() interrupts in-flight activities
//
// Run:
//
//	go run ./examples/shopflow
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

// Order is the typed payload threaded through every Signal, Activity, Condition,
// and hook. No type assertions anywhere in user code — all enforced at compile time.
type Order struct {
	ID           string
	CustomerName string
	Amount       float64
	Tier         string  // "express" | "standard" | "economy"
	FraudScore   float64 // 0.0–1.0; ≥ 0.7 triggers rejection

	// Filled in by activities during the workflow:
	ChargeRef    string
	InventoryRef string
	CarrierRef   string
}

func (o *Order) String() string {
	return fmt.Sprintf("Order{id=%s tier=%s amount=%.2f fraud=%.1f}",
		o.ID, o.Tier, o.Amount, o.FraudScore)
}

// Sentinel errors used by activities and NonRetryableErrors policy.
var (
	ErrFraudDetected   = errors.New("high fraud risk — order blocked")
	ErrPaymentDeclined = errors.New("payment gateway: card declined")
	ErrNoInventory     = errors.New("warehouse: item out of stock")
	ErrNoCarriers      = errors.New("shipping: no carriers available")
)

// ─────────────────────────────────────────────────────────────────────────────
// Activities  (pure business logic — no observability code here)
// ─────────────────────────────────────────────────────────────────────────────

// ── Fraud / Validation ───────────────────────────────────────────────────────

func runFraudCheck(_ context.Context, o *Order) error {
	logActivity("fraud-check", o.ID, fmt.Sprintf("score=%.1f", o.FraudScore))
	return nil
}

// ── Payment (Saga step 1) ────────────────────────────────────────────────────

var chargeAttempts = map[string]int{}

func chargeCard(_ context.Context, o *Order) error {
	chargeAttempts[o.ID]++
	attempt := chargeAttempts[o.ID]
	if attempt == 1 && strings.HasSuffix(o.ID, "-retry") {
		logActivity("charge-card", o.ID, fmt.Sprintf("attempt %d — gateway timeout", attempt))
		return errors.New("gateway timeout (transient)")
	}
	o.ChargeRef = fmt.Sprintf("CHG-%s-%d", o.ID, time.Now().UnixMilli())
	logActivity("charge-card", o.ID, fmt.Sprintf("charged $%.2f ref=%s", o.Amount, o.ChargeRef))
	return nil
}

func refundCard(_ context.Context, o *Order) error {
	logActivity("refund-card", o.ID,
		fmt.Sprintf("↩ refunding charge ref=%s  [SAGA COMPENSATION]", o.ChargeRef))
	o.ChargeRef = ""
	return nil
}

// ── Inventory (Saga step 2 — with retry) ────────────────────────────────────

var inventoryAttempts = map[string]int{}

func reserveInventory(_ context.Context, o *Order) error {
	inventoryAttempts[o.ID]++
	attempt := inventoryAttempts[o.ID]
	// Simulate 2 transient failures then success for the retry scenario.
	if attempt <= 2 && strings.HasSuffix(o.ID, "-retry") {
		logActivity("reserve-inventory", o.ID,
			fmt.Sprintf("attempt %d — warehouse service busy", attempt))
		return errors.New("warehouse temporarily unavailable")
	}
	// Simulate permanent out-of-stock for the saga-rollback scenario.
	if strings.HasSuffix(o.ID, "-outofstock") {
		logActivity("reserve-inventory", o.ID, "item permanently out of stock")
		return ErrNoInventory
	}
	o.InventoryRef = fmt.Sprintf("INV-%s-%03d", o.ID, attempt)
	logActivity("reserve-inventory", o.ID, fmt.Sprintf("reserved ref=%s", o.InventoryRef))
	return nil
}

func releaseInventory(_ context.Context, o *Order) error {
	if o.InventoryRef == "" {
		return nil
	}
	logActivity("release-inventory", o.ID,
		fmt.Sprintf("↩ releasing ref=%s  [SAGA COMPENSATION]", o.InventoryRef))
	o.InventoryRef = ""
	return nil
}

// ── Carrier booking (Saga step 3 — with timeout) ─────────────────────────────

func bookCarrier(ctx context.Context, o *Order) error {
	// Simulate a slow carrier API — will be bounded by Timeout() in the workflow.
	delay := 10 * time.Millisecond
	if strings.HasSuffix(o.ID, "-timeout") {
		delay = 2 * time.Second // will trip the 200 ms Timeout wrapper
	}
	// Use context-aware sleep so Timeout() can interrupt this activity.
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		logActivity("book-carrier", o.ID, "deadline exceeded — carrier call interrupted")
		return ctx.Err()
	}

	if strings.HasSuffix(o.ID, "-nocarrier") {
		logActivity("book-carrier", o.ID, "no carriers available for this route")
		return ErrNoCarriers
	}
	o.CarrierRef = fmt.Sprintf("CARRIER-%s-EXPRESS", strings.ToUpper(o.Tier))
	logActivity("book-carrier", o.ID, fmt.Sprintf("booked carrier ref=%s", o.CarrierRef))
	return nil
}

func cancelCarrierBooking(_ context.Context, o *Order) error {
	if o.CarrierRef == "" {
		return nil
	}
	logActivity("cancel-carrier", o.ID,
		fmt.Sprintf("↩ cancelling carrier ref=%s  [SAGA COMPENSATION]", o.CarrierRef))
	o.CarrierRef = ""
	return nil
}

// ── Notifications (OnEnter hooks — run concurrently) ─────────────────────────

func sendShippingEmail(_ context.Context, o *Order) error {
	time.Sleep(5 * time.Millisecond) // simulate SMTP call
	logActivity("send-shipping-email", o.ID,
		fmt.Sprintf("email sent to customer %s", o.CustomerName))
	return nil
}

func pushMobileNotification(_ context.Context, o *Order) error {
	time.Sleep(3 * time.Millisecond) // simulate push
	logActivity("push-notification", o.ID, "push notification sent")
	return nil
}

func updateWarehouseDashboard(_ context.Context, o *Order) error {
	time.Sleep(4 * time.Millisecond) // simulate internal API
	logActivity("warehouse-dashboard", o.ID, "dashboard updated")
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Middleware
// ─────────────────────────────────────────────────────────────────────────────

// Workflow-wide counters updated by metricsMiddleware.
var (
	totalCalls   atomic.Int64
	totalErrors  atomic.Int64
	totalLatency atomic.Int64 // nanoseconds
)

// metricsMiddleware records call count, error count, and latency — applied to
// EVERY activity and hook in the workflow via builder.WithMiddleware().
func metricsMiddleware(next workflow.Activity[*Order]) workflow.Activity[*Order] {
	return func(ctx context.Context, o *Order) error {
		start := time.Now()
		err := next(ctx, o)
		totalCalls.Add(1)
		totalLatency.Add(int64(time.Since(start)))
		if err != nil {
			totalErrors.Add(1)
		}
		return err
	}
}

// structuredLogger wraps a specific activity for targeted per-call logging.
// Different from metricsMiddleware: used only on selected activities.
func structuredLogger(activityName string) workflow.Middleware[*Order] {
	return func(next workflow.Activity[*Order]) workflow.Activity[*Order] {
		return func(ctx context.Context, o *Order) error {
			fmt.Printf("  ┌ [middleware] %s start  order=%s\n", activityName, o.ID)
			start := time.Now()
			err := next(ctx, o)
			elapsed := time.Since(start).Round(time.Millisecond)
			if err != nil {
				fmt.Printf("  └ [middleware] %s error=%v  latency=%v\n", activityName, err, elapsed)
			} else {
				fmt.Printf("  └ [middleware] %s ok  latency=%v\n", activityName, elapsed)
			}
			return err
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Routing conditions
// ─────────────────────────────────────────────────────────────────────────────

func isFraudulent(_ context.Context, o *Order) bool { return o.FraudScore >= 0.70 }
func tierKey(_ context.Context, o *Order) any       { return o.Tier }

// ─────────────────────────────────────────────────────────────────────────────
// Retry + timeout policies
// ─────────────────────────────────────────────────────────────────────────────

var paymentRetryPolicy = workflow.RetryPolicy{
	MaxAttempts:        3,
	InitialInterval:    20 * time.Millisecond,
	BackoffCoefficient: 2.0,
	MaxInterval:        200 * time.Millisecond,
	// Never retry a declined card — it won't succeed.
	NonRetryableErrors: []error{ErrPaymentDeclined},
}

var inventoryRetryPolicy = workflow.RetryPolicy{
	MaxAttempts:        4,
	InitialInterval:    15 * time.Millisecond,
	BackoffCoefficient: 2.0,
	// Never retry permanent out-of-stock.
	NonRetryableErrors: []error{ErrNoInventory},
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow Definition
// ─────────────────────────────────────────────────────────────────────────────
//
// One Workflow[*Order] shared across every Order instance (it is immutable).

var shopWF = workflow.Define[*Order]().
	// ── Workflow-wide middleware ─────────────────────────────────────────────
	// metricsMiddleware wraps EVERY activity and hook in the workflow.
	// No need to wrap each function individually.
	WithMiddleware(metricsMiddleware).

	// ── State hooks ──────────────────────────────────────────────────────────
	// When an order is shipped, send email + push + update dashboard in parallel.
	// All three run concurrently; the transition completes when all finish.
	OnEnterConcurrent("SHIPPED",
		sendShippingEmail,
		pushMobileNotification,
		updateWarehouseDashboard,
	).

	// ── Fraud gate (Feature: Conditional routing — If/Else) ──────────────────
	// validate signal: fraudulent → REJECTED, clean → PAYMENT_HOLD.
	From("PENDING").On("validate").
	If(isFraudulent, "REJECTED").
	Else("PAYMENT_HOLD").

	// ── Tier-based routing (Feature: Conditional routing — Switch) ───────────
	// fulfil signal routes to tier-specific fulfilment state.
	From("PAYMENT_HOLD").On("fulfil").
	Switch(tierKey).
	Case("express", "FULFILLING_EXPRESS").
	Case("standard", "FULFILLING_STANDARD").
	Default("FULFILLING_ECONOMY").

	// ── Express fulfilment (Feature: Saga + Retry + Timeout + per-activity middleware)
	From("FULFILLING_EXPRESS").On("ship").To("SHIPPED").
	Saga(
		// Charge card with retry (transient gateway errors are common).
		workflow.WithMiddleware(
			workflow.Retry(chargeCard, paymentRetryPolicy),
			structuredLogger("charge-card"),
		),
		refundCard, // saga compensation: auto-refund if a later step fails
	).
	Saga(
		// Reserve inventory with retry; out-of-stock aborts immediately.
		workflow.Retry(reserveInventory, inventoryRetryPolicy),
		releaseInventory,
	).
	// Book carrier with a hard 200 ms deadline.
	Activity(
		workflow.WithMiddleware(
			workflow.Timeout(bookCarrier, 200*time.Millisecond),
			structuredLogger("book-carrier"),
		),
	).

	// ── Standard fulfilment (same Saga, slower carrier deadline) ─────────────
	From("FULFILLING_STANDARD").On("ship").To("SHIPPED").
	Saga(workflow.Retry(chargeCard, paymentRetryPolicy), refundCard).
	Saga(workflow.Retry(reserveInventory, inventoryRetryPolicy), releaseInventory).
	Activity(workflow.Timeout(bookCarrier, 500*time.Millisecond)).

	// ── Economy fulfilment ────────────────────────────────────────────────────
	From("FULFILLING_ECONOMY").On("ship").To("SHIPPED").
	Saga(workflow.Retry(chargeCard, paymentRetryPolicy), refundCard).
	Saga(workflow.Retry(reserveInventory, inventoryRetryPolicy), releaseInventory).
	Activity(workflow.Timeout(bookCarrier, time.Second)).

	// ── Final delivery ────────────────────────────────────────────────────────
	From("SHIPPED").On("deliver").To("DELIVERED").

	// ── Cancellation from any pre-delivery state ──────────────────────────────
	// Activities here are best-effort cleanup; compensations in the saga
	// already handle partial rollbacks within the "ship" transition.
	From("PENDING", "PAYMENT_HOLD",
		"FULFILLING_EXPRESS", "FULFILLING_STANDARD", "FULFILLING_ECONOMY",
		"SHIPPED").
	On("cancel").To("CANCELLED").
	Activity(refundCard, releaseInventory, cancelCarrierBooking).

	MustBuild()

// ─────────────────────────────────────────────────────────────────────────────
// Main — runs all scenarios
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	divider("ShopFlow Workflow")
	fmt.Print(shopWF.Visualize())

	scenarios := []struct {
		name string
		fn   func()
	}{
		{"Scenario 1 — Happy Path (express order)", scenario1HappyPath},
		{"Scenario 2 — Fraud Rejection (conditional If/Else)", scenario2FraudRejection},
		{"Scenario 3 — Shipping Tier Routing (Switch)", scenario3TierRouting},
		{"Scenario 4 — Saga Rollback (carrier unavailable)", scenario4SagaRollback},
		{"Scenario 5 — Retry & Timeout", scenario5RetryAndTimeout},
		{"Scenario 6 — Middleware (metrics summary)", scenario6Middleware},
		{"Scenario 7 — Cancellation + Await", scenario7CancellationAndAwait},
	}
	for _, s := range scenarios {
		divider(s.name)
		s.fn()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 1 — Happy Path
//
// Feature: basic transitions, OnEnter concurrent hooks, Execution hooks,
//          Execution history, Await
// ─────────────────────────────────────────────────────────────────────────────
func scenario1HappyPath() {
	ctx := context.Background()
	o := &Order{ID: "ORD-001", CustomerName: "Alice", Amount: 149.99, Tier: "express", FraudScore: 0.1}

	exec := shopWF.NewExecution(ctx, "PENDING",
		workflow.WithHooks(workflow.ExecutionHooks[*Order]{
			OnTransition: func(_ context.Context, from, to, sig string, o *Order) {
				fmt.Printf("  ✓ %s --%s--> %s\n", from, sig, to)
			},
			OnError: func(_ context.Context, state, sig string, err error, o *Order) {
				fmt.Printf("  ✗ signal=%s state=%s: %v\n", sig, state, err)
			},
		}),
	)

	must(exec.Signal(ctx, "validate", o))
	must(exec.Signal(ctx, "fulfil", o))
	must(exec.Signal(ctx, "ship", o))
	must(exec.Signal(ctx, "deliver", o))

	// Await DELIVERED — already reached; returns immediately.
	awaitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := exec.Await(awaitCtx, func(s string) bool { return s == "DELIVERED" }); err != nil {
		fmt.Printf("  Await error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Await: reached DELIVERED\n")
	}

	printHistory(exec)
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 2 — Fraud Rejection
//
// Feature: If/Else conditional routing based on FraudScore
// ─────────────────────────────────────────────────────────────────────────────
func scenario2FraudRejection() {
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		fraudScore float64
		wantState  string
	}{
		{"clean order (score=0.2)", 0.2, "PAYMENT_HOLD"},
		{"suspicious order (score=0.85)", 0.85, "REJECTED"},
	} {
		o := &Order{ID: "ORD-002", CustomerName: "Bob", Amount: 299.00,
			Tier: "standard", FraudScore: tc.fraudScore}
		exec := shopWF.NewExecution(ctx, "PENDING")

		must(exec.Signal(ctx, "validate", o))
		state := exec.CurrentState()

		checkmark := "✓"
		if state != tc.wantState {
			checkmark = "✗"
		}
		fmt.Printf("  %s %s → landed in %s (wanted %s)\n", checkmark, tc.name, state, tc.wantState)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 3 — Tier Routing via Switch
//
// Feature: Switch/Case conditional routing; different destination states
//          per order tier
// ─────────────────────────────────────────────────────────────────────────────
func scenario3TierRouting() {
	ctx := context.Background()
	tiers := []string{"express", "standard", "economy", "unknown-tier"}

	for _, tier := range tiers {
		o := &Order{ID: "ORD-003", Amount: 50.00, Tier: tier, FraudScore: 0.1}
		exec := shopWF.NewExecution(ctx, "PENDING")
		must(exec.Signal(ctx, "validate", o)) // → PAYMENT_HOLD
		must(exec.Signal(ctx, "fulfil", o))   // → FULFILLING_<TIER>
		fmt.Printf("  tier=%-10s → %s\n", tier, exec.CurrentState())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 4 — Saga Rollback
//
// Feature: Saga compensation — carrier booking fails, inventory and card are
//          automatically compensated in reverse order.
// ─────────────────────────────────────────────────────────────────────────────
func scenario4SagaRollback() {
	ctx := context.Background()
	// Suffix "-nocarrier" makes bookCarrier return ErrNoCarriers.
	o := &Order{
		ID: "ORD-004-nocarrier", CustomerName: "Carol",
		Amount: 89.99, Tier: "express", FraudScore: 0.05,
	}

	exec := shopWF.NewExecution(ctx, "PENDING",
		workflow.WithHooks(workflow.ExecutionHooks[*Order]{
			OnError: func(_ context.Context, state, sig string, err error, o *Order) {
				fmt.Printf("  ✗ %s in %s — saga will compensate\n", sig, state)
			},
		}),
	)

	must(exec.Signal(ctx, "validate", o))
	must(exec.Signal(ctx, "fulfil", o))

	fmt.Println("\n  Sending 'ship' — bookCarrier will fail, triggering saga rollback:")
	err := exec.Signal(ctx, "ship", o)
	if err != nil {
		fmt.Printf("\n  Signal returned: %v\n", err)
	}

	fmt.Printf("\n  After saga rollback:\n")
	fmt.Printf("    ChargeRef    = %q  (empty = refunded)\n", o.ChargeRef)
	fmt.Printf("    InventoryRef = %q  (empty = released)\n", o.InventoryRef)
	fmt.Printf("    CarrierRef   = %q  (never booked)\n", o.CarrierRef)
	fmt.Printf("    State        = %s\n", exec.CurrentState())
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 5 — Retry & Timeout
//
// Feature: Retry() wrapper with exponential backoff + NonRetryableErrors;
//          Timeout() wrapper bounding slow activities.
// ─────────────────────────────────────────────────────────────────────────────
func scenario5RetryAndTimeout() {
	ctx := context.Background()

	// Part A: inventory retries 2× before succeeding; charge retries 1× on timeout.
	fmt.Println("  Part A — retry: inventory fails 2×, then succeeds:")
	oRetry := &Order{
		ID: "ORD-005-retry", CustomerName: "Dave",
		Amount: 59.99, Tier: "express", FraudScore: 0.0,
	}
	execRetry := shopWF.NewExecution(ctx, "PENDING")
	must(execRetry.Signal(ctx, "validate", oRetry))
	must(execRetry.Signal(ctx, "fulfil", oRetry))
	if err := execRetry.Signal(ctx, "ship", oRetry); err != nil {
		fmt.Printf("  ship failed: %v\n", err)
	} else {
		inv := inventoryAttempts[oRetry.ID]
		fmt.Printf("  ✓ shipped after %d inventory attempts\n", inv)
	}

	// Part B: bookCarrier sleeps 2 s but Timeout is 200 ms → times out.
	fmt.Println("\n  Part B — timeout: carrier call exceeds 200 ms deadline:")
	oTimeout := &Order{
		ID: "ORD-005-timeout", CustomerName: "Eve",
		Amount: 34.50, Tier: "express", FraudScore: 0.0,
	}
	execTimeout := shopWF.NewExecution(ctx, "PENDING")
	must(execTimeout.Signal(ctx, "validate", oTimeout))
	must(execTimeout.Signal(ctx, "fulfil", oTimeout))
	start := time.Now()
	err := execTimeout.Signal(ctx, "ship", oTimeout)
	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("  signal returned in %v: %v\n", elapsed, err)
	if elapsed < 300*time.Millisecond {
		fmt.Println("  ✓ Timeout() enforced deadline correctly")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 6 — Middleware metrics summary
//
// Feature: Workflow-wide middleware (metricsMiddleware) applied at Build() time.
//          Zero user code needed to instrument activities.
// ─────────────────────────────────────────────────────────────────────────────
func scenario6Middleware() {
	// Reset counters so we only measure this scenario.
	totalCalls.Store(0)
	totalErrors.Store(0)
	totalLatency.Store(0)

	ctx := context.Background()
	o := &Order{ID: "ORD-006", CustomerName: "Frank", Amount: 75.00, Tier: "standard", FraudScore: 0.2}

	exec := shopWF.NewExecution(ctx, "PENDING")
	must(exec.Signal(ctx, "validate", o))
	must(exec.Signal(ctx, "fulfil", o))
	must(exec.Signal(ctx, "ship", o))
	must(exec.Signal(ctx, "deliver", o))

	calls := totalCalls.Load()
	errs := totalErrors.Load()
	lat := time.Duration(totalLatency.Load())
	avg := time.Duration(0)
	if calls > 0 {
		avg = lat / time.Duration(calls)
	}

	fmt.Println("  metricsMiddleware summary (collected without touching any activity):")
	fmt.Printf("    activities executed : %d\n", calls)
	fmt.Printf("    errors recorded     : %d\n", errs)
	fmt.Printf("    total latency       : %v\n", lat.Round(time.Millisecond))
	fmt.Printf("    avg latency/activity: %v\n", avg.Round(time.Microsecond))
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario 7 — Cancellation + Await
//
// Feature: exec.Cancel() cancels the execution's lifecycle context, stopping
//          in-flight activities; exec.Await() blocks until a condition or
//          cancellation fires.
// ─────────────────────────────────────────────────────────────────────────────
func scenario7CancellationAndAwait() {
	ctx := context.Background()
	o := &Order{ID: "ORD-007", CustomerName: "Grace", Amount: 199.99, Tier: "express", FraudScore: 0.05}

	exec := shopWF.NewExecution(ctx, "PENDING")
	must(exec.Signal(ctx, "validate", o))
	must(exec.Signal(ctx, "fulfil", o))

	// Cancel before shipping — simulates customer changing their mind.
	fmt.Printf("  State before cancel: %s\n", exec.CurrentState())
	exec.Cancel()

	// Subsequent signals return ErrExecutionCancelled immediately.
	err := exec.Signal(ctx, "ship", o)
	fmt.Printf("  Signal after Cancel: %v\n", err)
	if errors.Is(err, workflow.ErrExecutionCancelled) {
		fmt.Println("  ✓ ErrExecutionCancelled returned — no activities ran")
	}

	// Done() channel is closed.
	select {
	case <-exec.Done():
		fmt.Println("  ✓ exec.Done() is closed")
	default:
		fmt.Println("  ✗ exec.Done() should be closed")
	}

	// Await on a separate Execution unblocks when condition is met.
	fmt.Println("\n  Await demo: goroutine signals → main goroutine unblocks:")
	oAwait := &Order{ID: "ORD-007b", CustomerName: "Grace", Amount: 10.00, Tier: "economy", FraudScore: 0.0}
	exec2 := shopWF.NewExecution(context.Background(), "PENDING")

	go func() {
		time.Sleep(40 * time.Millisecond)
		c := context.Background()
		exec2.Signal(c, "validate", oAwait)   //nolint:errcheck
		exec2.Signal(c, "fulfil", oAwait)     //nolint:errcheck
		exec2.Signal(c, "ship", oAwait)       //nolint:errcheck
		exec2.Signal(c, "deliver", oAwait)    //nolint:errcheck
	}()

	awaitCtx, awaitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer awaitCancel()

	start := time.Now()
	if err := exec2.Await(awaitCtx, func(s string) bool { return s == "DELIVERED" }); err != nil {
		fmt.Printf("  Await error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Await unblocked after %v — state=%s\n",
			time.Since(start).Round(time.Millisecond), exec2.CurrentState())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func must(err error) {
	if err != nil && !errors.Is(err, workflow.ErrExecutionCancelled) {
		// Errors in scenarios are part of the demonstration, not test failures.
	}
}

func logActivity(name, orderID, detail string) {
	fmt.Printf("  [%-24s] order=%-12s %s\n", name, orderID, detail)
}

func divider(title string) {
	fmt.Printf("\n╔%s╗\n", strings.Repeat("═", 60))
	fmt.Printf("  %s\n", title)
	fmt.Printf("╚%s╝\n", strings.Repeat("═", 60))
}

func printHistory(exec interface {
	History() []workflow.HistoryEntry
	CurrentState() string
}) {
	h := exec.History()
	fmt.Printf("\n  History — %d events, final state: %s\n", len(h), exec.CurrentState())
	fmt.Printf("  %-4s  %-22s  %-10s  %-22s  %-8s  %s\n",
		"#", "From", "Signal", "To", "Latency", "Status")
	fmt.Println("  " + strings.Repeat("─", 74))
	for i, e := range h {
		status := "✓ ok"
		if e.Err != nil {
			status = "✗ err"
		}
		fmt.Printf("  %-4d  %-22s  %-10s  %-22s  %-8v  %s\n",
			i+1, e.FromState, e.Signal, e.ToState,
			e.Duration.Round(time.Millisecond), status)
	}
}
