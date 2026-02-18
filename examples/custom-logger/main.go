// Custom-logger example: disable default logging or plug in your own logger.
//
// Run with: go run ./examples/custom-logger/main.go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Item state: IDLE -> RUNNING -> IDLE
type Item struct {
	Status string
}

func (i *Item) SetState(s statemachine.State) { i.Status = string(s) }
func (i *Item) GetState() statemachine.State   { return statemachine.State(i.Status) }

type RunEvent struct{}
func (RunEvent) GetEvent() statemachine.Event { return "run" }

type StopEvent struct{}
func (StopEvent) GetEvent() statemachine.Event { return "stop" }

func main() {
	ctx := context.Background()

	fmt.Println("=== With default logger (std log) ===")
	smDefault := buildMachine(nil)
	item1 := &Item{Status: "IDLE"}
	_, _ = smDefault.TriggerTransition(ctx, RunEvent{}, item1)
	fmt.Println("status:", item1.Status)
	fmt.Println()

	fmt.Println("=== With NoopLogger (no transition logs) ===")
	smQuiet := buildMachine(statemachine.NoopLogger{})
	item2 := &Item{Status: "IDLE"}
	_, _ = smQuiet.TriggerTransition(ctx, RunEvent{}, item2)
	fmt.Println("status:", item2.Status)
	fmt.Println()

	fmt.Println("=== With custom structured logger ===")
	smCustom := buildMachine(&structuredLogger{prefix: "[FSM]"})
	item3 := &Item{Status: "IDLE"}
	_, _ = smCustom.TriggerTransition(ctx, RunEvent{}, item3)
	fmt.Println("status:", item3.Status)
}

func buildMachine(logger statemachine.Logger) statemachine.StateMachine {
	opts := []statemachine.Option{}
	if logger != nil {
		opts = append(opts, statemachine.WithLogger(logger))
	}

	sm := statemachine.NewStatemachineWithOptions(
		statemachine.EventKey{Src: "IDLE", Event: "run"},
		opts...,
	)

	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"IDLE"}, Event: "run", Dst: "RUNNING",
	})
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"RUNNING"}, Event: "stop", Dst: "IDLE",
	})

	return sm
}

// structuredLogger implements statemachine.Logger with a prefix (e.g. for JSON or structured logs).
type structuredLogger struct {
	prefix string
}

func (s *structuredLogger) LogTransition(tr statemachine.Transition) {
	// Example: in production you might log as JSON or to a tracing system
	src := strings.Trim(strings.ReplaceAll(fmt.Sprint(tr.Src), " ", ","), "[]")
	fmt.Printf("%s %s --%s--> %s\n", s.prefix, src, tr.Event, tr.Dst)
}
