// Concurrent-execution example: running handlers concurrently for better performance.
//
// Run with: go run ./examples/concurrent-execution/main.go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Task that needs multiple initialization steps when entering a state.
type Task struct {
	ID          string
	Status      string
	Initialized bool
	Validated   bool
	Notified    bool
}

func (t *Task) SetState(s statemachine.State) { t.Status = string(s) }
func (t *Task) GetState() statemachine.State   { return statemachine.State(t.Status) }

type StartEvent struct{}
func (StartEvent) GetEvent() statemachine.Event { return "start" }

func main() {
	sm := buildTaskMachine()
	ctx := context.Background()

	task := &Task{ID: "task-1", Status: "CREATED"}

	fmt.Println("=== Sequential execution (default) ===")
	start := time.Now()
	_, err := sm.TriggerTransition(ctx, StartEvent{}, task)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	sequentialTime := time.Since(start)
	fmt.Printf("Status: %s, Initialized: %v, Validated: %v, Notified: %v\n", task.Status, task.Initialized, task.Validated, task.Notified)
	fmt.Printf("Time taken: %v\n\n", sequentialTime)

	// Reset for concurrent example
	task2 := &Task{ID: "task-2", Status: "CREATED"}
	sm2 := buildConcurrentTaskMachine()

	fmt.Println("=== Concurrent execution ===")
	start = time.Now()
	_, err = sm2.TriggerTransition(ctx, StartEvent{}, task2)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	concurrentTime := time.Since(start)
	fmt.Printf("Status: %s, Initialized: %v, Validated: %v, Notified: %v\n", task2.Status, task2.Initialized, task2.Validated, task2.Notified)
	fmt.Printf("Time taken: %v\n", concurrentTime)
	fmt.Printf("\nConcurrent execution was %.1fx faster\n", float64(sequentialTime)/float64(concurrentTime))
}

func buildTaskMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "CREATED", Event: "start"})
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"CREATED"}, Event: "start", Dst: "RUNNING",
	})

	// Sequential handlers (default)
	sm.AddStateEntry("RUNNING", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		time.Sleep(100 * time.Millisecond) // Simulate work
		task.Initialized = true
		fmt.Println("  Initialized")
		return nil
	})

	sm.AddStateEntry("RUNNING", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		time.Sleep(100 * time.Millisecond) // Simulate work
		task.Validated = true
		fmt.Println("  Validated")
		return nil
	})

	sm.AddStateEntry("RUNNING", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		time.Sleep(100 * time.Millisecond) // Simulate work
		task.Notified = true
		fmt.Println("  Notified")
		return nil
	})

	return sm
}

func buildConcurrentTaskMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "CREATED", Event: "start"})
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"CREATED"}, Event: "start", Dst: "RUNNING",
	})

	var mu sync.Mutex // Protect shared state

	// Concurrent handlers - run in parallel
	sm.AddStateEntryConcurrent("RUNNING", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		time.Sleep(100 * time.Millisecond) // Simulate work
		mu.Lock()
		task.Initialized = true
		mu.Unlock()
		fmt.Println("  Initialized (concurrent)")
		return nil
	})

	sm.AddStateEntryConcurrent("RUNNING", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		time.Sleep(100 * time.Millisecond) // Simulate work
		mu.Lock()
		task.Validated = true
		mu.Unlock()
		fmt.Println("  Validated (concurrent)")
		return nil
	})

	sm.AddStateEntryConcurrent("RUNNING", func(ctx context.Context, m statemachine.TransitionModel) error {
		task := m.(*Task)
		time.Sleep(100 * time.Millisecond) // Simulate work
		mu.Lock()
		task.Notified = true
		mu.Unlock()
		fmt.Println("  Notified (concurrent)")
		return nil
	})

	return sm
}
