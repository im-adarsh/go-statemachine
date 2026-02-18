package workflow

import "errors"

// Sentinel errors returned by Workflow and Execution methods.
// Use errors.Is() for matching:
//
//	if errors.Is(err, workflow.ErrRetryExhausted) { ... }
var (
	// ErrUnknownSignal is returned when no transition is registered for the
	// given signal in the current state.
	ErrUnknownSignal = errors.New("unknown signal for current state")

	// ErrNoConditionMatched is returned when a conditional (if-else or switch)
	// route is evaluated but no branch matches and no else/default clause is defined.
	ErrNoConditionMatched = errors.New("no condition matched and no else/default clause defined")

	// ErrRetryExhausted is returned by Retry() when all retry attempts are
	// exhausted and the Activity still returns an error.
	ErrRetryExhausted = errors.New("retry attempts exhausted")

	// ErrExecutionCancelled is returned by Execution.Signal when the Execution
	// has already been cancelled via Execution.Cancel().
	ErrExecutionCancelled = errors.New("execution has been cancelled")
)
