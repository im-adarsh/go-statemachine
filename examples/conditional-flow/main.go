// Conditional-flow example: multiple transitions from the same state+event
// with different guards (conditional flow chart).
//
// Run with: go run ./examples/conditional-flow/main.go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Order with priority; processing path depends on priority.
type Order struct {
	ID       string
	Priority string // "high", "medium", "low"
	Status   string
}

func (o *Order) SetState(s statemachine.State) { o.Status = string(s) }
func (o *Order) GetState() statemachine.State  { return statemachine.State(o.Status) }

type ProcessEvent struct{}
func (ProcessEvent) GetEvent() statemachine.Event { return "process" }

func main() {
	sm := buildOrderMachine()
	ctx := context.Background()

	// High priority order -> FAST_TRACK
	order1 := &Order{ID: "ord-1", Priority: "high", Status: "PENDING"}
	_, err := sm.TriggerTransition(ctx, ProcessEvent{}, order1)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Order %s (priority=%s) -> %s\n", order1.ID, order1.Priority, order1.Status)

	// Medium priority order -> STANDARD
	order2 := &Order{ID: "ord-2", Priority: "medium", Status: "PENDING"}
	_, err = sm.TriggerTransition(ctx, ProcessEvent{}, order2)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Order %s (priority=%s) -> %s\n", order2.ID, order2.Priority, order2.Status)

	// Low priority order -> SLOW_LANE (fallback, no guard)
	order3 := &Order{ID: "ord-3", Priority: "low", Status: "PENDING"}
	_, err = sm.TriggerTransition(ctx, ProcessEvent{}, order3)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Order %s (priority=%s) -> %s\n", order3.ID, order3.Priority, order3.Status)

	// Unknown priority -> SLOW_LANE (fallback)
	order4 := &Order{ID: "ord-4", Priority: "unknown", Status: "PENDING"}
	_, err = sm.TriggerTransition(ctx, ProcessEvent{}, order4)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Order %s (priority=%s) -> %s (fallback)\n", order4.ID, order4.Priority, order4.Status)
}

func buildOrderMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "PENDING", Event: "process"})

	// Conditional flow: same event, different destinations based on guard conditions
	// Transitions are evaluated in order; first matching guard wins

	// High priority -> fast track
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"PENDING"}, Event: "process", Dst: "FAST_TRACK",
		Guard: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
			order := m.(*Order)
			return order.Priority == "high", nil
		},
		Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
			fmt.Println("  -> Fast track processing")
			return nil, nil
		},
	})

	// Medium priority -> standard
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"PENDING"}, Event: "process", Dst: "STANDARD",
		Guard: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
			order := m.(*Order)
			return order.Priority == "medium", nil
		},
		Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
			fmt.Println("  -> Standard processing")
			return nil, nil
		},
	})

	// Fallback: no guard = always matches if previous guards fail
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"PENDING"}, Event: "process", Dst: "SLOW_LANE",
		Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
			fmt.Println("  -> Slow lane processing (fallback)")
			return nil, nil
		},
	})

	return sm
}
