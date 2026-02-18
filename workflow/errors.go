package workflow

import "errors"

// Sentinel errors returned by Workflow.Signal and Execution.Signal.
// Use errors.Is(err, workflow.ErrUnknownSignal) to test for specific failures.
var (
	// ErrUnknownSignal is returned when no transition is registered for the
	// current state + signal pair — analogous to sending a Signal to a Temporal
	// Workflow Execution that has no handler for it.
	ErrUnknownSignal = errors.New("unknown signal for current state")

	// ErrNoConditionMatched is returned when all Conditions in an if-else chain
	// fail with no Else clause, or a switch has no matching Case and no Default.
	ErrNoConditionMatched = errors.New("no condition matched and no else/default clause defined")
)
