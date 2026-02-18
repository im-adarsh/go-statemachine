// # Flowchart / Concurrent Executions Example — Loan Processing
//
// Uses go-statemachine as a parallel flowchart engine where many loan
// applications are processed concurrently, each driven by the same shared
// Workflow definition.
//
// Features demonstrated:
//
//   - Concurrent OnEnter hooks — credit check and fraud detection run in parallel
//   - Workflow-wide middleware — every activity is automatically wrapped with
//     a structured logger and a latency recorder
//   - Multiple concurrent Executions sharing one Workflow (safe — Workflow is immutable)
//   - Await — each goroutine blocks until its loan reaches a terminal state
//   - Execution.Cancel + Done — stalled applications are forcibly terminated
//   - Execution history per application — printed as a structured audit table
//
// Run:
//
//	go run ./examples/flowchart
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain
// ─────────────────────────────────────────────────────────────────────────────

type Loan struct {
	ID          string
	ApplicantID string
	Amount      float64
	Score       int // credit score 300–850
	Tier        string
}

func (l *Loan) riskLabel() string {
	switch {
	case l.Score >= 750:
		return "low-risk  (fast-track)"
	case l.Score >= 650:
		return "medium-risk (underwriting)"
	default:
		return "high-risk (auto-reject)"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Activities
// ─────────────────────────────────────────────────────────────────────────────

// creditCheck and fraudDetection run concurrently via OnEnterConcurrent.
func creditCheck(_ context.Context, l *Loan) error {
	time.Sleep(25 * time.Millisecond) // simulate external call
	return nil
}

func fraudDetection(_ context.Context, l *Loan) error {
	time.Sleep(20 * time.Millisecond)
	return nil
}

func underwriting(_ context.Context, l *Loan) error {
	time.Sleep(15 * time.Millisecond) // simulate underwriter review
	return nil
}

func disburseFunds(_ context.Context, l *Loan) error {
	fmt.Printf("  [%s] 💸 $%.0f disbursed to applicant %s\n", l.ID, l.Amount, l.ApplicantID)
	return nil
}

func sendRejection(_ context.Context, l *Loan) error {
	fmt.Printf("  [%s] 📧 rejection notice sent to applicant %s\n", l.ID, l.ApplicantID)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Routing conditions
// ─────────────────────────────────────────────────────────────────────────────

func isAutoReject(_ context.Context, l *Loan) bool { return l.Score < 600 }
func isFastTrack(_ context.Context, l *Loan) bool  { return l.Score >= 750 }

// ─────────────────────────────────────────────────────────────────────────────
// Middleware — applied workflow-wide at Build() time
// ─────────────────────────────────────────────────────────────────────────────

// Global counters updated by middleware.
var (
	totalActivitiesRun   atomic.Int64
	totalActivityLatency atomic.Int64 // nanoseconds
)

// metricsMiddleware records call count and latency for every activity.
func metricsMiddleware(next workflow.Activity[*Loan]) workflow.Activity[*Loan] {
	return func(ctx context.Context, l *Loan) error {
		start := time.Now()
		err := next(ctx, l)
		elapsed := time.Since(start)
		totalActivitiesRun.Add(1)
		totalActivityLatency.Add(int64(elapsed))
		return err
	}
}

// tracingMiddleware prints a structured log line for every activity call.
func tracingMiddleware(next workflow.Activity[*Loan]) workflow.Activity[*Loan] {
	return func(ctx context.Context, l *Loan) error {
		start := time.Now()
		err := next(ctx, l)
		status := "ok"
		if err != nil {
			status = fmt.Sprintf("err: %v", err)
		}
		fmt.Printf("  [%s] activity  status=%-4s  latency=%v\n",
			l.ID, status, time.Since(start).Round(time.Millisecond))
		return err
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow definition
// ─────────────────────────────────────────────────────────────────────────────

var loanWF = workflow.Define[*Loan]().
	// Apply both middleware layers to EVERY activity and hook.
	WithMiddleware(metricsMiddleware, tracingMiddleware).

	// On entry to SCREENING: run credit check and fraud detection in parallel.
	OnEnterConcurrent("SCREENING", creditCheck, fraudDetection).

	// Initial screening signal.
	From("PENDING").On("screen").To("SCREENING").

	// Risk assessment after screening.
	From("SCREENING").On("assess").
	If(isAutoReject, "REJECTED").
	ElseIf(isFastTrack, "FAST_TRACK").
	Else("UNDERWRITING").

	// Underwriting path (medium-risk loans).
	From("UNDERWRITING").On("decide").To("APPROVED").
	Activity(underwriting).

	// Fast-track path (low-risk: skip underwriting).
	From("FAST_TRACK").On("decide").To("APPROVED").

	// Disburse approved loans.
	From("APPROVED").On("disburse").To("DISBURSED").
	Activity(disburseFunds).

	// Reject high-risk loans.
	From("REJECTED").On("close").To("CLOSED").
	Activity(sendRejection).

	MustBuild()

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("┌─ Loan Workflow graph ───────────────────────────────────────")
	fmt.Print(loanWF.Visualize())
	fmt.Println("└────────────────────────────────────────────────────────────")
	fmt.Println()

	loans := []*Loan{
		{ID: "L-001", ApplicantID: "A-100", Amount: 5_000, Score: 780}, // fast-track
		{ID: "L-002", ApplicantID: "A-101", Amount: 25_000, Score: 680}, // underwriting
		{ID: "L-003", ApplicantID: "A-102", Amount: 1_000, Score: 550},  // auto-reject
		{ID: "L-004", ApplicantID: "A-103", Amount: 50_000, Score: 800}, // fast-track (large)
		{ID: "L-005", ApplicantID: "A-104", Amount: 8_000, Score: 620},  // underwriting
	}

	fmt.Printf("Processing %d loan applications concurrently...\n\n", len(loans))

	start := time.Now()
	var wg sync.WaitGroup
	for _, l := range loans {
		wg.Add(1)
		go func(loan *Loan) {
			defer wg.Done()
			if err := processLoan(loan); err != nil {
				fmt.Printf("[%s] ✗ processing error: %v\n", loan.ID, err)
			}
		}(l)
	}
	wg.Wait()

	elapsed := time.Since(start)
	runs := totalActivitiesRun.Load()
	latencyNs := totalActivityLatency.Load()

	fmt.Println()
	fmt.Println("┌─ Run summary ───────────────────────────────────────────────")
	fmt.Printf("│  Total elapsed           : %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("│  Activities executed     : %d\n", runs)
	if runs > 0 {
		avg := time.Duration(latencyNs / runs)
		fmt.Printf("│  Avg activity latency    : %v\n", avg.Round(time.Microsecond))
	}
	fmt.Println("└────────────────────────────────────────────────────────────")
}

// ─────────────────────────────────────────────────────────────────────────────
// processLoan drives one Loan through the workflow
// ─────────────────────────────────────────────────────────────────────────────

func processLoan(l *Loan) error {
	// Give each loan a 10-second window; cancel automatically if stalled.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exec := loanWF.NewExecution(ctx, "PENDING",
		workflow.WithHooks(workflow.ExecutionHooks[*Loan]{
			OnTransition: func(_ context.Context, from, to, signal string, l *Loan) {
				fmt.Printf("[%s] %s --%s--> %s\n", l.ID, from, signal, to)
			},
		}),
	)

	fmt.Printf("╔══ [%s] $%.0f  score=%d  %s\n", l.ID, l.Amount, l.Score, l.riskLabel())

	// Screen → assess → decide/reject → disburse/close
	steps := buildSteps(exec)
	for _, step := range steps {
		if exec.CurrentState() == step.requiredState {
			if err := exec.Signal(ctx, step.signal, l); err != nil {
				return fmt.Errorf("signal %q: %w", step.signal, err)
			}
		}
	}

	// Await a terminal state with a deadline.
	awaitCtx, awaitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer awaitCancel()
	terminal := func(s string) bool { return s == "DISBURSED" || s == "CLOSED" }
	if err := exec.Await(awaitCtx, terminal); err != nil {
		exec.Cancel() // forcibly abort if stalled
		return fmt.Errorf("await terminal: %w", err)
	}

	printAuditTable(l.ID, exec)
	return nil
}

// buildSteps returns the ordered signal sequence for a loan.
// The requiredState guard ensures each signal is only sent in the right state.
type signalStep struct {
	requiredState string
	signal        string
}

func buildSteps(exec interface{ CurrentState() string }) []signalStep {
	return []signalStep{
		{requiredState: "PENDING", signal: "screen"},
		{requiredState: "SCREENING", signal: "assess"},
		{requiredState: "FAST_TRACK", signal: "decide"},
		{requiredState: "UNDERWRITING", signal: "decide"},
		{requiredState: "REJECTED", signal: "close"},
		{requiredState: "APPROVED", signal: "disburse"},
	}
}

func printAuditTable(id string, exec interface {
	History() []workflow.HistoryEntry
	CurrentState() string
}) {
	h := exec.History()
	fmt.Printf("  ╚═ [%s] done → %-10s  (%d events)\n", id, exec.CurrentState(), len(h))
	fmt.Printf("     %-4s  %-12s  %-8s  %-12s  %s\n", "#", "From", "Signal", "To", "Latency")
	fmt.Println("     " + strings.Repeat("─", 50))
	for i, e := range h {
		fmt.Printf("     %-4d  %-12s  %-8s  %-12s  %v\n",
			i+1, e.FromState, e.Signal, e.ToState, e.Duration.Round(time.Millisecond))
	}
	fmt.Println()
}
