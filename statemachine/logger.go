package statemachine

import (
	"fmt"
	"log"
)

func logTrigger(tr Transition) {
	log.Println(fmt.Sprintf("[Current State : %v] -- %v --> [Destination State : %v]", tr.Src, tr.Event, tr.Dst))
}
