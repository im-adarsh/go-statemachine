package main

import (
	"fmt"

	fsm2 "github.com/looplab/fsm"
)

func main1() {
	fsm := fsm2.NewFSM(
		"green",
		fsm2.Events{
			{Name: "warn", Src: []string{"green"}, Dst: "yellow"},
		},
		fsm2.Callbacks{
			"before_warn": func(e *fsm2.Event) {
				fmt.Println("before_warn")
			},
			"before_event": func(e *fsm2.Event) {
				fmt.Println("before_event")
			},
			"leave_green": func(e *fsm2.Event) {
				fmt.Println("leave_green")
			},
			"leave_state": func(e *fsm2.Event) {
				fmt.Println("leave_state")
			},
			"enter_yellow": func(e *fsm2.Event) {
				fmt.Println("enter_yellow")
			},
			"enter_state": func(e *fsm2.Event) {
				fmt.Println("enter_state")
			},
			"after_warn": func(e *fsm2.Event) {
				fmt.Println("after_warn")
			},
			"after_event": func(e *fsm2.Event) {
				fmt.Println("after_event")
			},
		},
	)

	fmt.Println(fsm2.Visualize(fsm))
	fmt.Println(fsm.Current())
	err := fsm.Event("warn")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(fsm.Current())
}
