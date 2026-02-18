// Package main shows a realistic order-processing Workflow with:
//   - Transition Activities (validate, enrich)
//   - OnExit / OnEnter Activities (audit log)
//   - OnEnterParallel Activities (async post-processing)
//   - Conditional routing via If/ElseIf/Else (Conditions)
//   - Value-based routing via Switch/Case/Default (SwitchExpr)
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// Order is the payload that flows through every Activity.
type Order struct {
	ID     string
	Amount float64
	Method string // "card" | "paypal" | "crypto"
}

func main() {
	workflow, err := workflow.Define().
		// ── Simple transitions with Activities ───────────────────────
		From("CREATED").On("submit").To("SUBMITTED").
		Activity(validateOrder, auditSubmit).

		// ── Conditional routing via Conditions (if-else) ─────────────
		From("SUBMITTED").On("review").
		If(isHighValue).To("MANUAL_REVIEW").
		Else("APPROVED").

		// ── Value-based routing via SwitchExpr ────────────────────────
		From("APPROVED").On("pay").
		Switch(paymentMethod).
		Case("card", "CARD_PROCESSING").
		Case("paypal", "PAYPAL_PROCESSING").
		Default("UNSUPPORTED_PAYMENT").

		From("CARD_PROCESSING").On("capture").To("COMPLETED").
		From("PAYPAL_PROCESSING").On("capture").To("COMPLETED").
		From("MANUAL_REVIEW").On("approve").To("APPROVED").
		From("MANUAL_REVIEW").On("reject").To("REJECTED").

		// ── OnExit / OnEnter Activities ──────────────────────────────
		OnExit("CREATED", logExit("CREATED")).
		OnEnter("SUBMITTED", logEntry("SUBMITTED")).
		OnEnter("MANUAL_REVIEW", flagForReview).

		// ── Parallel OnEnter Activities ──────────────────────────────
		OnEnterParallel("COMPLETED",
			sendConfirmationEmail,
			updateInventory,
			notifyWarehouse,
		).
		WithLogger(workflow.DefaultLogger{}).
		Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(workflow.Visualize())

	order := &Order{ID: "ORD-001", Amount: 9_500, Method: "card"}
	ctx := context.Background()

	// Start an Execution — one per order.
	exec := workflow.NewExecution("CREATED")

	for _, signal := range []string{"submit", "review", "approve", "pay", "capture"} {
		fmt.Printf("\n[%s] ← signal %q\n", exec.CurrentState(), signal)
		if err := exec.Signal(ctx, signal, order); err != nil {
			log.Fatalf("  error: %v", err)
		}
		fmt.Printf("  → %s\n", exec.CurrentState())
	}
}

// ── Conditions ────────────────────────────────────────────────────────────────

func isHighValue(_ context.Context, payload any) bool {
	return payload.(*Order).Amount > 5_000
}

func paymentMethod(_ context.Context, payload any) any {
	return payload.(*Order).Method
}

// ── Transition Activities ─────────────────────────────────────────────────────

func validateOrder(_ context.Context, payload any) error {
	o := payload.(*Order)
	fmt.Printf("  validating order %s (amount=%.2f)\n", o.ID, o.Amount)
	return nil
}

func auditSubmit(_ context.Context, payload any) error {
	fmt.Printf("  audit: order %s submitted\n", payload.(*Order).ID)
	return nil
}

// ── OnExit / OnEnter Activities ───────────────────────────────────────────────

func logEntry(state string) workflow.Activity {
	return func(_ context.Context, _ any) error {
		fmt.Printf("  ↳ entered %s\n", state)
		return nil
	}
}

func logExit(state string) workflow.Activity {
	return func(_ context.Context, _ any) error {
		fmt.Printf("  ↳ exiting %s\n", state)
		return nil
	}
}

func flagForReview(_ context.Context, payload any) error {
	o := payload.(*Order)
	fmt.Printf("  ⚑ order %s sent to manual review (amount=%.2f)\n", o.ID, o.Amount)
	return nil
}

// ── Parallel OnEnter Activities (simulate async work) ─────────────────────────

func sendConfirmationEmail(_ context.Context, payload any) error {
	time.Sleep(10 * time.Millisecond)
	fmt.Printf("  ✉  confirmation email sent for %s\n", payload.(*Order).ID)
	return nil
}

func updateInventory(_ context.Context, payload any) error {
	time.Sleep(5 * time.Millisecond)
	fmt.Printf("  📦 inventory updated for %s\n", payload.(*Order).ID)
	return nil
}

func notifyWarehouse(_ context.Context, payload any) error {
	time.Sleep(8 * time.Millisecond)
	fmt.Printf("  🏭 warehouse notified for %s\n", payload.(*Order).ID)
	return nil
}
