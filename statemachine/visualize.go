package statemachine

import "fmt"

const START_LINE_DIVIDER = "\n\n######################################################"
const END_LINE_DIVIDER = "######################################################\n\n"

func Visualize(sm StateMachine) {
	if sm == nil {
		fmt.Println("cannot visualize uninitialized statemachine")
		return
	}

	_, trs := sm.GetTransitions()
	if trs == nil {
		fmt.Println("cannot visualize empty transitions")
	}

	srcToDstsMap := map[State][]EventKey{}
	for _, v := range trs {
		if _, ok := srcToDstsMap[v.Src]; !ok {
			srcToDstsMap[v.Src] = []EventKey{}
		}

		srcToDstsMap[v.Src] = append(srcToDstsMap[v.Src], EventKey{
			Src:   v.Dst,
			Event: v.Event,
		})
	}

	fmt.Println(START_LINE_DIVIDER)
	for k, vs := range srcToDstsMap {
		fmt.Println(fmt.Sprintf("| Node :  %v |", k))
		for _, v := range vs {
			fmt.Println(fmt.Sprintf("\t \t  -- %v --> | Node :  %v |", v.Event, v.Src))
		}
	}
	fmt.Println(END_LINE_DIVIDER)

}
