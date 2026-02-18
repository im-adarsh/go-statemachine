package workflow

import "log"

// Logger receives a notification each time a Signal successfully drives a
// Workflow Execution from one state to another.
type Logger interface {
	LogTransition(from, signal, to string)
}

// DefaultLogger writes each transition to the standard log package.
type DefaultLogger struct{}

func (DefaultLogger) LogTransition(from, signal, to string) {
	log.Printf("[workflow] %s --%s--> %s", from, signal, to)
}

// NoopLogger discards all log output. Useful in tests or high-throughput paths.
type NoopLogger struct{}

func (NoopLogger) LogTransition(from, signal, to string) {}
