# go-statemachine
###### Motivation : https://github.com/Tinder/StateMachine

![Image of Statemachine](static/activity-diagram.png)

```
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

type onMelt struct {
}

func (receiver *onMelt) GetEvent() statemachine.Event {
	return "onMelt"
}

type onVapourise struct {
}

func (receiver *onVapourise) GetEvent() statemachine.Event {
	return "onVapourise"
}

type onUnknownEvent struct {
}

func (receiver *onUnknownEvent) GetEvent() statemachine.Event {
	return "onUnknownEvent"
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
	_, err := sm.TriggerTransition(context.Background(), &onMelt{}, pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}
	fmt.Println("after onMelt : ", pr.Status)
	fmt.Println()

	// LIQUID -> GAS
	_, err = sm.TriggerTransition(context.Background(), &onVapourise{}, pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}

	fmt.Println("after onVapourise : ", pr.Status)
	fmt.Println()

	// GAS -> UNKNOWN (unregistered event)
	_, err = sm.TriggerTransition(context.Background(), &onUnknownEvent{}, pr)
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
		Src:        []statemachine.State{"SOLID"},
		Event:      "onMelt",
		Dst:        "LIQUID",
		Transition: onEvent(),
	})
	sm.AddTransition(statemachine.Transition{
		Src:              []statemachine.State{"LIQUID"},
		Event:            "onVapourise",
		Dst:              "GAS",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	sm.AddTransition(statemachine.Transition{
		Src:              []statemachine.State{"GAS"},
		Event:            "onCondensation",
		Dst:              "LIQUID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	sm.AddTransition(statemachine.Transition{
		Src:              []statemachine.State{"LIQUID"},
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
	return func(context.Context, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println("before")
		return nil, nil
	}
}

func onEvent() statemachine.TransitionHandler {
	return func(context.Context, statemachine.TransitionEvent, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println("during")
		return nil, nil
	}
}

func onAfterEvent() statemachine.AfterTransitionHandler {
	return func(context.Context, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println("after")
		return nil, nil
	}
}

func onSuccess() statemachine.OnSuccessHandler {
	return func(context.Context, statemachine.TransitionModel) (statemachine.TransitionModel, error) {
		fmt.Println("success")
		return nil, nil
	}
}

func onFailure() statemachine.OnFailureHandler {
	return func(context.Context, statemachine.TransitionModel, statemachine.Error, error) (statemachine.TransitionModel, error) {
		fmt.Println("failure")
		return nil, nil
	}
}

```

## Output
```
######################################################
| Node :  LIQUID |
                  -- onVapourise --> | Node :  GAS |
                  -- onFreeze --> | Node :  SOLID |
| Node :  GAS |
                  -- onCondensation --> | Node :  LIQUID |
| Node :  SOLID |
                  -- onMelt --> | Node :  LIQUID |
######################################################

2021/02/03 15:12:06 [Current State : [SOLID]] -- onMelt --> [Destination State : LIQUID]
during
after onMelt :  LIQUID

2021/02/03 15:12:06 [Current State : [LIQUID]] -- onVapourise --> [Destination State : GAS]
before
during
after
success
after onVapourise :  GAS

error :  transition is not defined

Process finished with exit code 0

```

[![BuyMeACoffee](https://bmc-cdn.nyc3.digitaloceanspaces.com/BMC-button-images/custom_images/orange_img.png)](https://www.buymeacoffee.com/imadarsh)