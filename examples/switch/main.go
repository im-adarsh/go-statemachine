// Switch example: using SwitchBlock for switch-case flow chart patterns.
//
// Run with: go run ./examples/switch/main.go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Payment with method; routing depends on payment method.
type Payment struct {
	ID      string
	Method  string // "credit_card", "paypal", "bank_transfer", "crypto"
	Status  string
	Message string
}

func (p *Payment) SetState(s statemachine.State) { p.Status = string(s) }
func (p *Payment) GetState() statemachine.State  { return statemachine.State(p.Status) }

type ProcessPaymentEvent struct{}
func (ProcessPaymentEvent) GetEvent() statemachine.Event { return "process" }

func main() {
	sm := buildPaymentMachine()
	ctx := context.Background()

	methods := []string{"credit_card", "paypal", "bank_transfer", "crypto", "unknown"}

	for _, method := range methods {
		payment := &Payment{ID: fmt.Sprintf("pay-%s", method), Method: method, Status: "PENDING"}
		fmt.Printf("Processing payment %s (method=%s)\n", payment.ID, payment.Method)
		_, err := sm.TriggerTransition(ctx, ProcessPaymentEvent{}, payment)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("Result: %s -> %s", payment.ID, payment.Status)
		if payment.Message != "" {
			fmt.Printf(" (%s)", payment.Message)
		}
		fmt.Println("\n")
	}
}

func buildPaymentMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "PENDING", Event: "process"})

	// Switch flow chart pattern using SwitchBlock
	sm.AddSwitchBlock(statemachine.SwitchBlock{
		Src:   []statemachine.State{"PENDING"},
		Event: "process",
		// Extract value to switch on
		SwitchExpr: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (interface{}, error) {
			payment := m.(*Payment)
			return payment.Method, nil
		},
		Cases: []statemachine.SwitchCase{
			// case "credit_card":
			{
				Value: "credit_card",
				Transition: statemachine.Transition{
					Src: []statemachine.State{"PENDING"},
					Dst: "PROCESSING_CARD",
					Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
						payment := m.(*Payment)
						payment.Message = "Processing credit card"
						return nil, nil
					},
				},
			},
			// case "paypal":
			{
				Value: "paypal",
				Transition: statemachine.Transition{
					Src: []statemachine.State{"PENDING"},
					Dst: "PROCESSING_PAYPAL",
					Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
						payment := m.(*Payment)
						payment.Message = "Redirecting to PayPal"
						return nil, nil
					},
				},
			},
			// case "bank_transfer":
			{
				Value: "bank_transfer",
				Transition: statemachine.Transition{
					Src: []statemachine.State{"PENDING"},
					Dst: "AWAITING_TRANSFER",
					Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
						payment := m.(*Payment)
						payment.Message = "Waiting for bank transfer"
						return nil, nil
					},
				},
			},
			// case "crypto":
			{
				Value: "crypto",
				Transition: statemachine.Transition{
					Src: []statemachine.State{"PENDING"},
					Dst: "PROCESSING_CRYPTO",
					Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
						payment := m.(*Payment)
						payment.Message = "Processing cryptocurrency"
						return nil, nil
					},
				},
			},
		},
		// default:
		DefaultTransition: &statemachine.Transition{
			Src: []statemachine.State{"PENDING"},
			Dst: "UNSUPPORTED",
			Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
				payment := m.(*Payment)
				payment.Message = "Unsupported payment method"
				return nil, nil
			},
		},
	})

	return sm
}
