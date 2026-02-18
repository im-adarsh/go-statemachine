// If-else example: using ConditionBlock for if-else flow chart patterns.
//
// Run with: go run ./examples/if-else/main.go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Order with amount; processing path depends on amount.
type Order struct {
	ID     string
	Amount float64
	Status string
}

func (o *Order) SetState(s statemachine.State) { o.Status = string(s) }
func (o *Order) GetState() statemachine.State  { return statemachine.State(o.Status) }

type ProcessEvent struct{}
func (ProcessEvent) GetEvent() statemachine.Event { return "process" }

func main() {
	sm := buildOrderMachine()
	ctx := context.Background()

	// High value order -> PREMIUM
	order1 := &Order{ID: "ord-1", Amount: 10000, Status: "PENDING"}
	fmt.Printf("Processing order %s (amount=$%.2f)\n", order1.ID, order1.Amount)
	_, err := sm.TriggerTransition(ctx, ProcessEvent{}, order1)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Result: %s -> %s\n\n", order1.ID, order1.Status)

	// Medium value order -> STANDARD
	order2 := &Order{ID: "ord-2", Amount: 5000, Status: "PENDING"}
	fmt.Printf("Processing order %s (amount=$%.2f)\n", order2.ID, order2.Amount)
	_, err = sm.TriggerTransition(ctx, ProcessEvent{}, order2)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Result: %s -> %s\n\n", order2.ID, order2.Status)

	// Low value order -> BASIC (else clause)
	order3 := &Order{ID: "ord-3", Amount: 500, Status: "PENDING"}
	fmt.Printf("Processing order %s (amount=$%.2f)\n", order3.ID, order3.Amount)
	_, err = sm.TriggerTransition(ctx, ProcessEvent{}, order3)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Result: %s -> %s (else)\n", order3.ID, order3.Status)
}

func buildOrderMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "PENDING", Event: "process"})

	// If-else flow chart pattern using ConditionBlock
	sm.AddConditionBlock(statemachine.ConditionBlock{
		Src:   []statemachine.State{"PENDING"},
		Event: "process",
		Cases: []statemachine.ConditionCase{
			// if amount >= 10000
			{
				Condition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
					order := m.(*Order)
					return order.Amount >= 10000, nil
				},
				Transition: statemachine.Transition{
					Src: []statemachine.State{"PENDING"},
					Dst: "PREMIUM",
					Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
						fmt.Println("  -> Premium processing (if amount >= $10000)")
						return nil, nil
					},
				},
			},
			// else if amount >= 1000
			{
				Condition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
					order := m.(*Order)
					return order.Amount >= 1000, nil
				},
				Transition: statemachine.Transition{
					Src: []statemachine.State{"PENDING"},
					Dst: "STANDARD",
					Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
						fmt.Println("  -> Standard processing (else if amount >= $1000)")
						return nil, nil
					},
				},
			},
		},
		// else
		ElseTransition: &statemachine.Transition{
			Src: []statemachine.State{"PENDING"},
			Dst: "BASIC",
			Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
				fmt.Println("  -> Basic processing (else)")
				return nil, nil
			},
		},
	})

	return sm
}
