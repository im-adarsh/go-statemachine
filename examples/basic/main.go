// # Basic Example — Traffic Light
//
// Demonstrates the fundamental building blocks of go-statemachine:
//
//   - Define[T]()         — type-safe workflow definition
//   - MustBuild()         — compile the workflow
//   - NewExecution()      — create a stateful instance
//   - Signal()            — drive state transitions
//   - WithHooks()         — lifecycle callbacks (OnTransition / OnError)
//   - History()           — inspect the full event log
//   - Await()             — block until a condition is met
//   - Visualize()         — human-readable graph
//
// Run:
//
//	go run ./examples/basic
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain
// ─────────────────────────────────────────────────────────────────────────────

// TrafficLight is the typed payload passed through every signal.
// Because the workflow is parameterised on *TrafficLight, every Activity
// receives a *TrafficLight — no type assertion ever required.
type TrafficLight struct {
	ID       string
	Location string
	Cycles   int
}

// ─────────────────────────────────────────────────────────────────────────────
// Activities
// ─────────────────────────────────────────────────────────────────────────────

// recordCycle is run on every entry into RED (completing one full cycle).
func recordCycle(_ context.Context, tl *TrafficLight) error {
	tl.Cycles++
	fmt.Printf("  ↺  Cycle %d complete at %s\n", tl.Cycles, tl.Location)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow definition (build once; share across every traffic light)
// ─────────────────────────────────────────────────────────────────────────────

var trafficLightWF = workflow.Define[*TrafficLight]().
	WithLogger(workflow.NoopLogger{}).
	// Standard traffic-light cycle: RED → GREEN → YELLOW → RED
	From("RED").On("next").To("GREEN").
	From("GREEN").On("next").To("YELLOW").
	From("YELLOW").On("next").To("RED").
	// Record a completed cycle each time we re-enter RED.
	OnEnter("RED", recordCycle).
	MustBuild()

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	ctx := context.Background()

	// ── Visualise the graph ───────────────────────────────────────────────────
	fmt.Println("┌─ Workflow graph ────────────────────────────────────────────")
	fmt.Print(trafficLightWF.Visualize())
	fmt.Println("└────────────────────────────────────────────────────────────")
	fmt.Println()

	// ── Create a stateful Execution ───────────────────────────────────────────
	tl := &TrafficLight{ID: "TL-42", Location: "Main St & 1st Ave"}

	exec := trafficLightWF.NewExecution(ctx, "RED",
		workflow.WithHooks(workflow.ExecutionHooks[*TrafficLight]{
			OnTransition: func(_ context.Context, from, to, signal string, tl *TrafficLight) {
				color := ansiColor(to)
				fmt.Printf("  %s[%s]%s  %s  --%s-->  %s\n",
					color, tl.ID, ansiReset, from, signal, to)
			},
			OnError: func(_ context.Context, state, signal string, err error, tl *TrafficLight) {
				fmt.Printf("  ✗ [%s] signal=%q state=%q error=%v\n", tl.ID, signal, state, err)
			},
		}),
	)

	fmt.Printf("Initial state: %s\n\n", exec.CurrentState())
	fmt.Println("── Cycling through 2 complete loops ─────────────────────────")

	// Drive 6 transitions (2 complete RED→GREEN→YELLOW→RED cycles).
	for i := 0; i < 6; i++ {
		if err := exec.Signal(ctx, "next", tl); err != nil {
			log.Fatalf("Signal: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // simulate real timing
	}

	// ── Await a specific state (already reached; returns immediately) ─────────
	fmt.Println()
	awaitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	fmt.Println("── Awaiting RED state ────────────────────────────────────────")
	if err := exec.Await(awaitCtx, func(state string) bool { return state == "RED" }); err != nil {
		log.Fatalf("Await: %v", err)
	}
	fmt.Printf("Condition met — currently in: %s\n", exec.CurrentState())

	// ── Print history ─────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("── Event history ─────────────────────────────────────────────")
	h := exec.History()
	fmt.Printf("%-4s  %-8s  %-8s  %-8s  %s\n", "Seq", "From", "Signal", "To", "Duration")
	fmt.Println(repeat("─", 52))
	for i, e := range h {
		status := "✓"
		if e.Err != nil {
			status = "✗"
		}
		fmt.Printf("%-4d  %-8s  %-8s  %-8s  %s  %s\n",
			i+1, e.FromState, e.Signal, e.ToState,
			e.Duration.Round(time.Microsecond), status)
	}
	fmt.Printf("\nTotal transitions: %d   Cycles completed: %d\n", len(h), tl.Cycles)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

const ansiReset = "\033[0m"

func ansiColor(state string) string {
	switch state {
	case "RED":
		return "\033[31m"
	case "GREEN":
		return "\033[32m"
	case "YELLOW":
		return "\033[33m"
	default:
		return ""
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
