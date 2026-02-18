// Phase example: state machine for phase transitions (SOLID ↔ LIQUID ↔ GAS)
// with lifecycle hooks and visualization.
//
// Run with: go run ./examples/phase/main.go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Phase holds the current phase (state) of matter.
type Phase struct {
	Label string
}

func (p *Phase) SetState(s statemachine.State) { p.Label = string(s) }
func (p *Phase) GetState() statemachine.State   { return statemachine.State(p.Label) }

// Event types for phase transitions.
type OnMelt struct{}
func (OnMelt) GetEvent() statemachine.Event { return "onMelt" }

type OnFreeze struct{}
func (OnFreeze) GetEvent() statemachine.Event { return "onFreeze" }

type OnVapourise struct{}
func (OnVapourise) GetEvent() statemachine.Event { return "onVapourise" }

type OnCondensation struct{}
func (OnCondensation) GetEvent() statemachine.Event { return "onCondensation" }

type OnUnknownEvent struct{}
func (OnUnknownEvent) GetEvent() statemachine.Event { return "onUnknownEvent" }

func main() {
	sm := buildPhaseMachine()

	// Print a text diagram of the state machine.
	fmt.Println("State machine diagram:")
	statemachine.Visualize(sm)
	fmt.Println()

	phase := &Phase{Label: "SOLID"}
	ctx := context.Background()

	// SOLID -> LIQUID
	if _, err := sm.TriggerTransition(ctx, OnMelt{}, phase); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("after Melt:", phase.Label)

	// LIQUID -> GAS
	if _, err := sm.TriggerTransition(ctx, OnVapourise{}, phase); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("after Vapourise:", phase.Label)

	// GAS -> LIQUID
	if _, err := sm.TriggerTransition(ctx, OnCondensation{}, phase); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("after Condensation:", phase.Label)

	// LIQUID -> SOLID
	if _, err := sm.TriggerTransition(ctx, OnFreeze{}, phase); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("after Freeze:", phase.Label)

	// Invalid: no transition for (SOLID, onUnknownEvent)
	_, err := sm.TriggerTransition(ctx, OnUnknownEvent{}, phase)
	if err != nil {
		fmt.Println("expected error for unknown event:", err)
	}
}

func buildPhaseMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "SOLID", Event: "onMelt"})

	// SOLID --onMelt--> LIQUID (no hooks)
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"SOLID"}, Event: "onMelt", Dst: "LIQUID",
		Transition: logTransition("during"),
	})

	// LIQUID --onVapourise--> GAS (full lifecycle)
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"LIQUID"}, Event: "onVapourise", Dst: "GAS",
		BeforeTransition: logBefore("before"),
		Transition:       logTransition("during"),
		AfterTransition:  logAfter("after"),
		OnSuccess:        logOnSuccess("success"),
		OnFailure:        logPhaseFailure("failure"),
	})

	// GAS --onCondensation--> LIQUID
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"GAS"}, Event: "onCondensation", Dst: "LIQUID",
		BeforeTransition: logBefore("before"),
		Transition:       logTransition("during"),
		AfterTransition:  logAfter("after"),
		OnSuccess:        logOnSuccess("success"),
		OnFailure:        logPhaseFailure("failure"),
	})

	// LIQUID --onFreeze--> SOLID
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"LIQUID"}, Event: "onFreeze", Dst: "SOLID",
		BeforeTransition: logBefore("before"),
		Transition:       logTransition("during"),
		AfterTransition:  logAfter("after"),
		OnSuccess:        logOnSuccess("success"),
		OnFailure:        logPhaseFailure("failure"),
	})

	return sm
}

func logBefore(msg string) statemachine.BeforeTransitionHandler {
	return func(context.Context, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println(msg)
		return nil, nil
	}
}

func logAfter(msg string) statemachine.AfterTransitionHandler {
	return func(context.Context, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println(msg)
		return nil, nil
	}
}

func logOnSuccess(msg string) statemachine.OnSuccessHandler {
	return func(context.Context, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println(msg)
		return nil, nil
	}
}

func logTransition(msg string) statemachine.TransitionHandler {
	return func(context.Context, statemachine.TransitionEvent, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println(msg)
		return nil, nil
	}
}

func logPhaseFailure(msg string) statemachine.OnFailureHandler {
	return func(context.Context, statemachine.TransitionModel, statemachine.Error, error) (statemachine.TransitionModel, error) {
		fmt.Println(msg)
		return nil, nil
	}
}
