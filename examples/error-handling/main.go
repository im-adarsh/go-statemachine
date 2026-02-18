// Error-handling example: using errors.Is for sentinel errors and ErrIgnore
// to abort a transition without changing state.
//
// Run with: go run ./examples/error-handling/main.go
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Ticket state: OPEN -> IN_PROGRESS -> DONE (or stay OPEN on validation failure).
type Ticket struct {
	ID     string
	Status string
}

func (t *Ticket) SetState(s statemachine.State) { t.Status = string(s) }
func (t *Ticket) GetState() statemachine.State   { return statemachine.State(t.Status) }

type StartEvent struct{}
func (StartEvent) GetEvent() statemachine.Event { return "start" }

type FinishEvent struct{}
func (FinishEvent) GetEvent() statemachine.Event { return "finish" }

type InvalidEvent struct{}
func (InvalidEvent) GetEvent() statemachine.Event { return "invalid" }

func main() {
	sm := buildTicketMachine()
	ctx := context.Background()

	// 1. Valid transition: OPEN -> IN_PROGRESS
	ticket := &Ticket{ID: "T-1", Status: "OPEN"}
	_, err := sm.TriggerTransition(ctx, StartEvent{}, ticket)
	if err != nil {
		fmt.Println("unexpected error:", err)
		return
	}
	fmt.Println("after start:", ticket.Status)

	// 2. Undefined transition: IN_PROGRESS + "invalid" event
	_, err = sm.TriggerTransition(ctx, InvalidEvent{}, ticket)
	if err != nil {
		if errors.Is(err, statemachine.ErrUndefinedTransition) {
			fmt.Println("correctly detected undefined transition")
		}
		fmt.Println("error:", err)
	}
	fmt.Println("status unchanged:", ticket.Status)

	// 3. Trigger "finish" which fails validation; OnFailure returns ErrIgnore so state does not change
	_, err = sm.TriggerTransition(ctx, FinishEvent{}, ticket)
	if err != nil {
		fmt.Println("unexpected error:", err)
		return
	}
	// State remains IN_PROGRESS because we returned ErrIgnore in OnFailure
	fmt.Println("after finish (validation failed, ErrIgnore):", ticket.Status)
}

func buildTicketMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "OPEN", Event: "start"})

	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"OPEN"}, Event: "start", Dst: "IN_PROGRESS",
	})

	// Simulate validation failure in Transition; OnFailure returns ErrIgnore so we abort without changing state
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"IN_PROGRESS"}, Event: "finish", Dst: "DONE",
		Transition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
			// Simulate validation failure
			return nil, errors.New("validation failed: missing required field")
		},
		OnFailure: func(ctx context.Context, m statemachine.TransitionModel, _ statemachine.Error, err error) (statemachine.TransitionModel, error) {
			fmt.Println("onFailure called:", err)
			// Swallow error and abort transition: state is NOT changed to DONE
			return m, statemachine.ErrIgnore
		},
	})

	return sm
}
