# go-statemachine

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

func main() {
	sm := statemachine.NewStatemachine(statemachine.EventKey{
		Src:   "SOLID",
		Event: "onMelt",
	})

	// initialize statemachine
	sm.AddTransition(statemachine.Transition{
		Src:              "SOLID",
		Event:            "onMelt",
		Dst:              "LIQUID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
	})

	sm.AddTransition(statemachine.Transition{
		Src:              "LIQUID",
		Event:            "onVapourise",
		Dst:              "GAS",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
	})

	sm.AddTransition(statemachine.Transition{
		Src:              "GAS",
		Event:            "onCondensation",
		Dst:              "LIQUID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
	})

	sm.AddTransition(statemachine.Transition{
		Src:              "LIQUID",
		Event:            "onFreeze",
		Dst:              "SOLID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
	})

	// visualize statemachine
	statemachine.Visualize(sm)

	// start to trigger the statemachine
	pr := &Purchase{
		PurchaseId: "p_123",
		Status:     "",
	}
	evtKey := statemachine.EventKey{Src: "SOLID", Event: "onMelt"}
	err := sm.TriggerTransition(context.Background(), evtKey, pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}
	fmt.Println("after onMelt : ", pr.Status)

	evtKey = statemachine.EventKey{Src: "LIQUID", Event: "onVapourise"}
	err = sm.TriggerTransition(context.Background(), evtKey, pr)
	if err != nil {
		fmt.Println("error : ", err)
		return
	}

	fmt.Println("after onVapourise : ", pr.Status)
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
```

## Output
```
######################################################
SOLID -- onMelt --> LIQUID
LIQUID -- onVapourise --> GAS
GAS -- onCondensation --> LIQUID
LIQUID -- onFreeze --> SOLID
######################################################


before
during
after
after onMelt :  LIQUID
before
during
after
after onVapourise :  GAS


```