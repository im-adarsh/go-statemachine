// Package main shows the minimal go-statemachine usage:
// define a Workflow, start an Execution, send Signals.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/im-adarsh/go-statemachine/workflow"
)

func main() {
	// Define the Workflow — build once, reuse across many Executions.
	workflow, err := workflow.Define().
		From("SOLID").On("melt").To("LIQUID").
		From("LIQUID").On("vaporize").To("GAS").
		From("GAS").On("condense").To("LIQUID").
		From("LIQUID").On("freeze").To("SOLID").
		WithLogger(workflow.DefaultLogger{}).
		Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(workflow.Visualize())

	// Start a new Execution at "SOLID".
	exec := workflow.NewExecution("SOLID")

	ctx := context.Background()
	for _, signal := range []string{"melt", "vaporize", "condense", "freeze"} {
		if err := exec.Signal(ctx, signal, nil); err != nil {
			log.Fatalf("signal %q failed: %v", signal, err)
		}
		fmt.Printf("  → %s\n", exec.CurrentState())
	}
}
