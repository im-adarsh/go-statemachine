package statemachine

import "fmt"

const START_LINE_DIVIDER = "\n\n######################################################"
const END_LINE_DIVIDER = "######################################################\n\n"

func Visualize(sm StateMachine) {

	if sm == nil {
		fmt.Println("cannot visualize uninitialized statemachine")
	}
	_, trs := sm.GetTransitions()

	fmt.Println(START_LINE_DIVIDER)
	for _, v := range trs {
		fmt.Println(fmt.Sprintf("%v -- %v --> %v", v.Src, v.Event, v.Dst))
	}
	fmt.Println(END_LINE_DIVIDER)

}
