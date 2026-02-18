// # Order Processing Example
//
// A realistic e-commerce order workflow that demonstrates:
//
//   - Conditional routing  — If/ElseIf/Else (approval gate) and Switch (tier routing)
//   - Saga compensation    — charge card + deduct inventory with automatic refund on failure
//   - Retry with backoff   — inventory deduction retried up to 3× with exponential backoff
//   - Activity timeout     — third-party shipping call bounded to 2 s
//   - Execution hooks      — OnTransition (audit log) + OnError (alert)
//   - Execution history    — full event log at completion
//   - Await                — block until a terminal state is reached
//   - Cancellation         — stalled orders cancelled via exec.Cancel()
//
// Happy path:  PENDING → APPROVED → VIP_FULFIL → SHIPPED → DELIVERED
// Sad path:    PENDING → REJECTED  (low-value, auto-rejected)
//
// Run:
//
//	go run ./examples/order
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain
// ─────────────────────────────────────────────────────────────────────────────

type Order struct {
	ID       string
	Amount   float64
	Tier     string // "premium" | "standard" | "budget"
	Approved bool
	// Internal tracking set by activities:
	ChargeRef   string
	InventoryOK bool
}

// ─────────────────────────────────────────────────────────────────────────────
// Sentinel errors
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrPaymentDeclined = errors.New("payment declined by issuer")
	ErrFraudBlock      = errors.New("order blocked by fraud detection")
)

// ─────────────────────────────────────────────────────────────────────────────
// Activities — approval gate
// ─────────────────────────────────────────────────────────────────────────────

func validateOrder(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] ✓ order validated (amount=%.2f tier=%s)\n", o.ID, o.Amount, o.Tier)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Activities — fulfilment (Saga steps)
// ─────────────────────────────────────────────────────────────────────────────

func chargeCard(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] 💳 charging $%.2f...\n", o.ID, o.Amount)
	o.ChargeRef = fmt.Sprintf("CHG-%s-%d", o.ID, time.Now().UnixMilli())
	fmt.Printf("  [%s] ✓ charged (ref=%s)\n", o.ID, o.ChargeRef)
	return nil
}

func refundCard(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] ↩ refunding charge ref=%s (saga compensation)\n", o.ID, o.ChargeRef)
	o.ChargeRef = ""
	return nil
}

var inventoryAttempts = map[string]int{}

func deductInventory(_ context.Context, o *Order) error {
	inventoryAttempts[o.ID]++
	attempt := inventoryAttempts[o.ID]
	fmt.Printf("  [%s] 📦 deducting inventory (attempt %d)...\n", o.ID, attempt)
	if attempt < 3 {
		return fmt.Errorf("inventory service unavailable (attempt %d)", attempt)
	}
	o.InventoryOK = true
	fmt.Printf("  [%s] ✓ inventory reserved\n", o.ID)
	return nil
}

func restoreInventory(_ context.Context, o *Order) error {
	if o.InventoryOK {
		fmt.Printf("  [%s] ↩ restoring inventory (saga compensation)\n", o.ID)
		o.InventoryOK = false
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Activities — shipping
// ─────────────────────────────────────────────────────────────────────────────

func scheduleVIPShipping(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] 🚀 scheduling VIP same-day shipping\n", o.ID)
	return nil
}

func scheduleStandardShipping(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] 📮 scheduling standard 3-5 day shipping\n", o.ID)
	return nil
}

func scheduleBudgetShipping(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] 📬 scheduling budget 7-10 day shipping\n", o.ID)
	return nil
}

func notifyCustomer(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] 📧 shipping confirmation sent to customer\n", o.ID)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Activities — rejection
// ─────────────────────────────────────────────────────────────────────────────

func sendRejectionEmail(_ context.Context, o *Order) error {
	fmt.Printf("  [%s] 📧 rejection email sent\n", o.ID)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Routing conditions
// ─────────────────────────────────────────────────────────────────────────────

func isApproved(_ context.Context, o *Order) bool { return o.Approved }
func tierKey(_ context.Context, o *Order) any     { return o.Tier }

// ─────────────────────────────────────────────────────────────────────────────
// Retry + timeout policies
// ─────────────────────────────────────────────────────────────────────────────

var inventoryRetry = workflow.RetryPolicy{
	MaxAttempts:        3,
	InitialInterval:    20 * time.Millisecond,
	BackoffCoefficient: 2.0, // 20ms → 40ms → 80ms
	MaxInterval:        500 * time.Millisecond,
	// Fraud blocks are terminal — never retry.
	NonRetryableErrors: []error{ErrFraudBlock},
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow definition
// ─────────────────────────────────────────────────────────────────────────────

var orderWF = workflow.Define[*Order]().
	WithLogger(workflow.DefaultLogger{}).

	// ── Step 1: Validation (OnExit from PENDING) ─────────────────────────────
	OnExit("PENDING", validateOrder).

	// ── Step 2: Approval gate — If/Else conditional routing ──────────────────
	From("PENDING").On("review").
	If(isApproved, "APPROVED").
	Else("REJECTED").

	// ── Step 3: Rejection path ────────────────────────────────────────────────
	From("REJECTED").On("notify").To("CLOSED").
	Activity(sendRejectionEmail).

	// ── Step 4: Tier-based routing using Switch ───────────────────────────────
	From("APPROVED").On("fulfil").
	Switch(tierKey).
	Case("premium", "VIP_FULFIL").
	Case("standard", "STANDARD_FULFIL").
	Default("BUDGET_FULFIL"). // catches "budget" and any unknown tier

	// ── Step 5: VIP fulfilment — Saga + Retry + Timeout ──────────────────────
	From("VIP_FULFIL").On("ship").To("SHIPPED").
	Saga(chargeCard, refundCard).                                         // saga: auto-refund on failure
	Saga(workflow.Retry(deductInventory, inventoryRetry), restoreInventory). // saga: retry + auto-restore
	Activity(workflow.Timeout(scheduleVIPShipping, 2*time.Second)).

	// ── Step 5a: Standard fulfilment ─────────────────────────────────────────
	From("STANDARD_FULFIL").On("ship").To("SHIPPED").
	Saga(chargeCard, refundCard).
	Saga(workflow.Retry(deductInventory, inventoryRetry), restoreInventory).
	Activity(workflow.Timeout(scheduleStandardShipping, 2*time.Second)).

	// ── Step 5b: Budget fulfilment ────────────────────────────────────────────
	From("BUDGET_FULFIL").On("ship").To("SHIPPED").
	Saga(chargeCard, refundCard).
	Saga(workflow.Retry(deductInventory, inventoryRetry), restoreInventory).
	Activity(workflow.Timeout(scheduleBudgetShipping, 2*time.Second)).

	// ── Step 6: Delivery confirmation ─────────────────────────────────────────
	From("SHIPPED").On("deliver").To("DELIVERED").
	Activity(notifyCustomer).

	MustBuild()

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("┌─ Order Workflow graph ──────────────────────────────────────")
	fmt.Print(orderWF.Visualize())
	fmt.Println("└────────────────────────────────────────────────────────────")
	fmt.Println()

	orders := []*Order{
		{ID: "ORD-001", Amount: 249.99, Tier: "premium", Approved: true},
		{ID: "ORD-002", Amount: 39.99, Tier: "standard", Approved: true},
		{ID: "ORD-003", Amount: 5.00, Tier: "budget", Approved: false}, // auto-rejected
	}

	for _, o := range orders {
		fmt.Printf("╔══ Processing %s (tier=%s  approved=%v) ══\n", o.ID, o.Tier, o.Approved)
		if err := processOrder(o); err != nil {
			log.Printf("  ✗ %s failed: %v\n", o.ID, err)
		}
		fmt.Println()
	}
}

func processOrder(o *Order) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	exec := orderWF.NewExecution(ctx, "PENDING",
		workflow.WithHooks(workflow.ExecutionHooks[*Order]{
			OnTransition: func(_ context.Context, from, to, signal string, o *Order) {
				fmt.Printf("  → [%s] %s --%s--> %s\n", o.ID, from, signal, to)
			},
			OnError: func(_ context.Context, state, signal string, err error, o *Order) {
				fmt.Printf("  ✗ [%s] error in state=%s signal=%s: %v\n", o.ID, state, signal, err)
			},
		}),
	)

	// ── Approval review ───────────────────────────────────────────────────────
	if err := exec.Signal(ctx, "review", o); err != nil {
		return fmt.Errorf("review: %w", err)
	}

	// ── Branch: rejected orders get a notification then stop ─────────────────
	if exec.CurrentState() == "REJECTED" {
		if err := exec.Signal(ctx, "notify", o); err != nil {
			return fmt.Errorf("notify: %w", err)
		}
		printHistory(exec)
		return nil
	}

	// ── Tier routing ──────────────────────────────────────────────────────────
	if err := exec.Signal(ctx, "fulfil", o); err != nil {
		return fmt.Errorf("fulfil: %w", err)
	}

	// ── Ship ──────────────────────────────────────────────────────────────────
	if err := exec.Signal(ctx, "ship", o); err != nil {
		return fmt.Errorf("ship: %w", err)
	}

	// ── Deliver ───────────────────────────────────────────────────────────────
	if err := exec.Signal(ctx, "deliver", o); err != nil {
		return fmt.Errorf("deliver: %w", err)
	}

	// ── Wait until DELIVERED (already there; returns immediately) ─────────────
	awaitCtx, awaitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer awaitCancel()
	if err := exec.Await(awaitCtx, func(s string) bool { return s == "DELIVERED" }); err != nil {
		return fmt.Errorf("await: %w", err)
	}

	printHistory(exec)
	return nil
}

func printHistory(exec interface {
	History() []workflow.HistoryEntry
	CurrentState() string
}) {
	h := exec.History()
	fmt.Printf("\n  History (%d events) — final state: %s\n", len(h), exec.CurrentState())
	fmt.Printf("  %-4s  %-14s  %-8s  %-14s  %-8s  %s\n",
		"#", "From", "Signal", "To", "Duration", "Status")
	fmt.Println("  " + strings.Repeat("─", 60))
	for i, e := range h {
		status := "✓ ok"
		if e.Err != nil {
			status = "✗ err"
		}
		fmt.Printf("  %-4d  %-14s  %-8s  %-14s  %-8s  %s\n",
			i+1, e.FromState, e.Signal, e.ToState,
			e.Duration.Round(time.Millisecond), status)
	}
}
