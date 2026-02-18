// Basic example: minimal state machine with a single transition and no hooks.
//
// Run with: go run ./examples/basic/main.go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Order is our transition model: it holds the current status as state.
type Order struct {
	ID     string
	Status string
}

func (o *Order) SetState(s statemachine.State) { o.Status = string(s) }
func (o *Order) GetState() statemachine.State   { return statemachine.State(o.Status) }

// SubmitEvent triggers the DRAFT -> PENDING transition.
type SubmitEvent struct{}

func (SubmitEvent) GetEvent() statemachine.Event { return "submit" }

func main() {
	// 1. Create machine with an initial “entry” event key (for reference; e.g. first transition).
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "DRAFT", Event: "submit"})

	// 2. Define the only transition: from DRAFT, on "submit", go to PENDING.
	_ = sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"DRAFT"},
		Event: "submit",
		Dst:  "PENDING",
	})

	order := &Order{ID: "ord-1", Status: "DRAFT"}
	ctx := context.Background()

	// 3. Trigger the transition.
	_, err := sm.TriggerTransition(ctx, SubmitEvent{}, order)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("order status:", order.Status) // PENDING
}
