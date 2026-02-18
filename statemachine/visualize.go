package statemachine

import "fmt"

const StartLineDivider = "\n\n######################################################"
const EndLineDivider = "######################################################\n\n"

// Visualize prints a text representation of the state machine's transitions to stdout.
// Safe to call with nil or empty state machines (prints a message and returns).
func Visualize(sm StateMachine) {
	if sm == nil {
		fmt.Println("cannot visualize uninitialized statemachine")
		return
	}

	_, trs := sm.GetTransitions()
	if trs == nil || len(trs) == 0 {
		fmt.Println("cannot visualize empty transitions")
		return
	}

	srcToDstsMap := map[State][]EventKey{}
	for _, v := range trs {
		for _, src := range v.Src {
			if _, ok := srcToDstsMap[src]; !ok {
				srcToDstsMap[src] = []EventKey{}
			}

			srcToDstsMap[src] = append(srcToDstsMap[src], EventKey{
				Src:   v.Dst,
				Event: v.Event,
			})
		}
	}

	fmt.Println(StartLineDivider)
	for k, vs := range srcToDstsMap {
		fmt.Printf("| Node :  %v |\n", k)
		for _, v := range vs {
			fmt.Printf("\t \t  -- %v --> | Node :  %v |\n", v.Event, v.Src)
		}
	}
	fmt.Print(EndLineDivider)

}
