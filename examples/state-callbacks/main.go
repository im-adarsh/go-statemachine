// State-callbacks example: state entry and exit handlers.
//
// Run with: go run ./examples/state-callbacks/main.go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Task lifecycle: TODO -> IN_PROGRESS -> DONE
type Task struct {
	ID          string
	Status      string
	StartedAt   string
	CompletedAt string
}

func (t *Task) SetState(s statemachine.State) { t.Status = string(s) }
func (t *Task) GetState() statemachine.State   { return statemachine.State(t.Status) }

type StartEvent struct{}
func (StartEvent) GetEvent() statemachine.Event { return "start" }

type CompleteEvent struct{}
func (CompleteEvent) GetEvent() statemachine.Event { return "complete" }

func main() {
	sm := buildTaskMachine()
	ctx := context.Background()

	task := &Task{ID: "task-1", Status: "TODO"}

	fmt.Println("=== Starting task ===")
	_, err := sm.TriggerTransition(ctx, StartEvent{}, task)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Task status: %s, StartedAt: %s\n\n", task.Status, task.StartedAt)

	fmt.Println("=== Completing task ===")
	_, err = sm.TriggerTransition(ctx, CompleteEvent{}, task)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Task status: %s, CompletedAt: %s\n", task.Status, task.CompletedAt)
}

func buildTaskMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "TODO", Event: "start"})

	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"TODO"}, Event: "start", Dst: "IN_PROGRESS",
	})

	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"IN_PROGRESS"}, Event: "complete", Dst: "DONE",
	})

	// State exit: TODO -> IN_PROGRESS
	sm.AddStateExit("TODO", func(ctx context.Context, m statemachine.TransitionModel) error {
		fmt.Println("Exiting TODO state")
		return nil
	})

	// State entry: entering IN_PROGRESS
	sm.AddStateEntry("IN_PROGRESS", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		task.StartedAt = "2026-02-18T10:00:00Z"
		fmt.Println("Entering IN_PROGRESS state: recording start time")
		return nil
	})

	// State exit: IN_PROGRESS -> DONE
	sm.AddStateExit("IN_PROGRESS", func(ctx context.Context, m statemachine.TransitionModel) error {
		fmt.Println("Exiting IN_PROGRESS state")
		return nil
	})

	// State entry: entering DONE
	sm.AddStateEntry("DONE", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		task.CompletedAt = "2026-02-18T11:00:00Z"
		fmt.Println("Entering DONE state: recording completion time")
		return nil
	})

	return sm
}
