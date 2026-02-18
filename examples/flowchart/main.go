// Package main demonstrates go-statemachine as a flow chart engine:
// one shared Workflow drives many concurrent Executions, each routed
// differently based on the payload — exactly like Temporal running many
// Workflow Executions from a single Workflow Definition.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// Application is the payload threaded through every Activity.
type Application struct {
	ID       int
	Score    int    // credit score
	Decision string // set by the human reviewer: "approve" | "reject"
}

func main() {
	// Define the Workflow once — shared across all concurrent Executions.
	workflow := workflow.Define().
		// ── Condition-based routing (if-else) ────────────────────────
		From("RECEIVED").On("evaluate").
		If(highScore).To("AUTO_APPROVED").
		ElseIf(mediumScore).To("MANUAL_REVIEW").
		Else("AUTO_REJECTED").

		// ── SwitchExpr-based routing (switch-case) ───────────────────
		From("MANUAL_REVIEW").On("decide").
		Switch(reviewDecision).
		Case("approve", "APPROVED").
		Case("reject", "REJECTED").
		Default("PENDING_INFO").

		From("AUTO_APPROVED", "APPROVED").On("disburse").To("FUNDED").
		From("AUTO_REJECTED", "REJECTED").On("notify").To("CLOSED").
		From("PENDING_INFO").On("resubmit").To("RECEIVED").

		// ── OnEnter Activities ────────────────────────────────────────
		OnEnter("AUTO_APPROVED", func(_ context.Context, p any) error {
			a := p.(*Application)
			fmt.Printf("  [%d] ✓ auto-approved (score=%d)\n", a.ID, a.Score)
			return nil
		}).
		OnEnter("AUTO_REJECTED", func(_ context.Context, p any) error {
			a := p.(*Application)
			fmt.Printf("  [%d] ✗ auto-rejected (score=%d)\n", a.ID, a.Score)
			return nil
		}).
		OnEnter("MANUAL_REVIEW", func(_ context.Context, p any) error {
			a := p.(*Application)
			fmt.Printf("  [%d] ⚑ queued for review (score=%d)\n", a.ID, a.Score)
			return nil
		}).
		MustBuild()

	fmt.Println(workflow.Visualize())

	applications := []*Application{
		{ID: 1, Score: 780}, // → AUTO_APPROVED
		{ID: 2, Score: 620}, // → MANUAL_REVIEW (standard → approve)
		{ID: 3, Score: 480}, // → AUTO_REJECTED
		{ID: 4, Score: 710}, // → MANUAL_REVIEW (express → approve)
		{ID: 5, Score: 550}, // → AUTO_REJECTED  (score 550 < 600, goes to AUTO_REJECTED)
		{ID: 6, Score: 800}, // → AUTO_APPROVED
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	for _, app := range applications {
		wg.Add(1)
		go func(a *Application) {
			defer wg.Done()

			// Each Application gets its own independent Execution.
			exec := workflow.NewExecution("RECEIVED")

			if err := exec.Signal(ctx, "evaluate", a); err != nil {
				log.Printf("[%d] evaluate: %v", a.ID, err)
				return
			}

			switch exec.CurrentState() {
			case "MANUAL_REVIEW":
				// Simulate a human reviewer: approve even IDs.
				if a.ID%2 == 0 {
					a.Decision = "approve"
				} else {
					a.Decision = "reject"
				}
				if err := exec.Signal(ctx, "decide", a); err != nil {
					log.Printf("[%d] decide: %v", a.ID, err)
					return
				}
				if exec.CanReceive("disburse") {
					exec.Signal(ctx, "disburse", a) //nolint:errcheck
				} else {
					exec.Signal(ctx, "notify", a) //nolint:errcheck
				}
			case "AUTO_APPROVED":
				exec.Signal(ctx, "disburse", a) //nolint:errcheck
			case "AUTO_REJECTED":
				exec.Signal(ctx, "notify", a) //nolint:errcheck
			}

			fmt.Printf("  [%d] final state: %s\n", a.ID, exec.CurrentState())
		}(app)
	}

	wg.Wait()
	fmt.Println("\nAll applications processed.")
}

// ── Conditions ────────────────────────────────────────────────────────────────

func highScore(_ context.Context, p any) bool   { return p.(*Application).Score >= 720 }
func mediumScore(_ context.Context, p any) bool  { return p.(*Application).Score >= 600 }

// ── SwitchExpr ────────────────────────────────────────────────────────────────

func reviewDecision(_ context.Context, p any) any { return p.(*Application).Decision }
