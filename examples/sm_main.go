package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

type Purchase struct {
	PurchaseId string
	Status     string
}

func (p *Purchase) SetState(s statemachine.State) {
	p.Status = string(s)
}

func (p *Purchase) GetState() statemachine.State {
	return statemachine.State(p.Status)
}

func main() {

	// create statemachine
	sm := createStatemachine()

	// visualize statemachine
	statemachine.Visualize(sm)

	// start to trigger the statemachine
	pr := &Purchase{
		PurchaseId: "p_123",
		Status:     "SOLID",
	}

	// SOLID -> LIQUID
	err := sm.TriggerTransition(context.Background(), "onMelt", pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}
	fmt.Println("after onMelt : ", pr.Status)
	fmt.Println()

	// LIQUID -> GAS
	err = sm.TriggerTransition(context.Background(), "onVapourise", pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}

	fmt.Println("after onVapourise : ", pr.Status)
	fmt.Println()

	// GAS -> UNKNOWN (unregistered event)
	err = sm.TriggerTransition(context.Background(), "onUnknownEvent", pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}

	fmt.Println("after onUnknownEvent : ", pr.Status)
	fmt.Println()
}

// initialize statemachine
func createStatemachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{
		Src:   "SOLID",
		Event: "onMelt",
	})
	// add state
	sm.AddTransition(statemachine.Transition{
		Src:        "SOLID",
		Event:      "onMelt",
		Dst:        "LIQUID",
		Transition: onEvent(),
	})
	sm.AddTransition(statemachine.Transition{
		Src:              "LIQUID",
		Event:            "onVapourise",
		Dst:              "GAS",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	sm.AddTransition(statemachine.Transition{
		Src:              "GAS",
		Event:            "onCondensation",
		Dst:              "LIQUID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	sm.AddTransition(statemachine.Transition{
		Src:              "LIQUID",
		Event:            "onFreeze",
		Dst:              "SOLID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	return sm
}

func onBeforeEvent() statemachine.BeforeTransitionHandler {
	return func(context.Context, statemachine.TransitionModel) error {
		fmt.Println("before")
		return nil
	}
}

func onEvent() statemachine.TransitionHandler {
	return func(context.Context, statemachine.TransitionModel) error {
		fmt.Println("during")
		return nil
	}
}

func onAfterEvent() statemachine.AfterTransitionHandler {
	return func(context.Context, statemachine.TransitionModel) error {
		fmt.Println("after")
		return nil
	}
}

func onSuccess() statemachine.OnSuccessHandler {
	return func(context.Context, statemachine.TransitionModel) error {
		fmt.Println("success")
		return nil
	}
}

func onFailure() statemachine.OnFailureHandler {
	return func(context.Context, statemachine.TransitionModel, statemachine.Error, error) error {
		fmt.Println("failure")
		return nil
	}
}
