package statemachine

import "log"

// Logger is used to log transition attempts. Implement this interface to
// plug in custom logging (e.g. structured logs or no-op in production).
type Logger interface {
	LogTransition(tr Transition)
}

// defaultLogger uses the standard log package.
type defaultLogger struct{}

func (defaultLogger) LogTransition(tr Transition) {
	log.Printf("[Current State: %v] -- %v --> [Destination State: %v]", tr.Src, tr.Event, tr.Dst)
}

// NoopLogger discards all log output. Use with WithLogger(statemachine.NoopLogger{}) to disable logging.
type NoopLogger struct{}

func (NoopLogger) LogTransition(Transition) {}
