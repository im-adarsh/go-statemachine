# go-statemachine

A minimal, production-ready **finite state machine** library for Go. Define states, events, and transitions with optional lifecycle hooks—ideal for order workflows, approval flows, and any state-driven logic.

[![Go Reference](https://pkg.go.dev/badge/github.com/im-adarsh/go-statemachine.svg)](https://pkg.go.dev/github.com/im-adarsh/go-statemachine/statemachine)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Inspired by [Tinder/StateMachine](https://github.com/Tinder/StateMachine).

---

## Table of contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Concepts](#concepts)
- [Quick start](#quick-start)
- [Examples](#examples)
- [Lifecycle hooks](#lifecycle-hooks)
- [Error handling](#error-handling)
- [Configuration](#configuration)
- [API overview](#api-overview)
- [Testing](#testing)
- [License](#license)

---

## Features

| Feature | Description |
|--------|-------------|
| **Simple API** | Define transitions as (source state(s), event, destination state). No code generation. |
| **Lifecycle hooks** | `BeforeTransition`, `Transition`, `AfterTransition`, `OnSuccess`, `OnFailure`—all optional. |
| **Guard conditions** | Conditional transitions: only execute if `Guard` returns true (e.g., balance check). |
| **Conditional flow charts** | Multiple transitions from same (state, event) with different guards—first matching guard wins. |
| **If-else blocks** | `AddConditionBlock` for if-else flow chart patterns—conditions evaluated in order, first match executes. |
| **Switch blocks** | `AddSwitchBlock` for switch-case flow chart patterns—match a value and route to different transitions. |
| **Concurrent execution** | Run state entry/exit handlers concurrently for better performance (`AddStateEntryConcurrent`, `AddStateExitConcurrent`). |
| **Parallel transitions** | `TriggerParallelTransitions` to execute multiple transitions concurrently. |
| **State callbacks** | `AddStateEntry` and `AddStateExit` for actions when entering/exiting states. |
| **Thread-safe** | Safe for concurrent use with multiple goroutines (mutex-protected). |
| **Handler return values** | Handlers may return an updated model; the machine uses it for subsequent steps and as the final result. |
| **Sentinel errors** | Use `errors.Is(err, statemachine.ErrUndefinedTransition)` and similar for robust error handling. |
| **ErrIgnore** | Return `statemachine.ErrIgnore` from `OnFailure` to swallow the error and abort the transition **without** changing state. |
| **Configurable logging** | Plug in a custom `Logger` or use `NoopLogger{}` to disable logs. |
| **Visualize** | Print a text diagram of the state machine for docs or debugging. |

---

## Requirements

- **Go 1.21+**

---

## Installation

```bash
go get github.com/im-adarsh/go-statemachine/statemachine
```

```go
import "github.com/im-adarsh/go-statemachine/statemachine"
```

---

## Concepts

- **State** — A value (e.g. `"PENDING"`, `"SHIPPED"`) held by your model.
- **Event** — A trigger (e.g. `"Submit"`, `"Ship"`) supplied when firing a transition.
- **Transition** — A rule: from one or more source states, on an event, move to a destination state. You can attach optional hooks to each transition.
- **Model** — Your struct that implements `TransitionModel` (`GetState` / `SetState`). The machine reads and updates its state.
- **Event key** — The pair `(SourceState, Event)` that uniquely identifies which transition to run.

---

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

type Order struct{ Status string }

func (o *Order) SetState(s statemachine.State) { o.Status = string(s) }
func (o *Order) GetState() statemachine.State   { return statemachine.State(o.Status) }

func main() {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "DRAFT", Event: "submit"})
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"DRAFT"}, Event: "submit", Dst: "PENDING",
	})

	order := &Order{Status: "DRAFT"}
	_, err := sm.TriggerTransition(context.Background(), event("submit"), order)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("status:", order.Status) // PENDING
}

type event string
func (e event) GetEvent() statemachine.Event { return statemachine.Event(e) }
```

---

## Examples

The [examples/](examples/) directory contains runnable programs. Run any of them with:

```bash
go run ./examples/<name>/main.go
```

| Example | Description |
|--------|-------------|
| [basic](examples/basic/main.go) | Minimal FSM: one transition, no hooks. |
| [phase](examples/phase/main.go) | Phase diagram (SOLID ↔ LIQUID ↔ GAS) with before/during/after hooks and visualization. |
| [error-handling](examples/error-handling/main.go) | Using `errors.Is` for undefined transitions and `ErrIgnore` to abort without changing state. |
| [custom-logger](examples/custom-logger/main.go) | Disable or customize logging with `WithLogger`. |
| [guard](examples/guard/main.go) | Conditional transitions using guard conditions (e.g., balance check before withdrawal). |
| [state-callbacks](examples/state-callbacks/main.go) | State entry and exit callbacks for actions when entering/exiting states. |
| [conditional-flow](examples/conditional-flow/main.go) | Multiple transitions from same state+event with guards (conditional flow chart). |
| [concurrent-execution](examples/concurrent-execution/main.go) | Running handlers concurrently for better performance. |
| [if-else](examples/if-else/main.go) | If-else flow chart pattern using `ConditionBlock`. |
| [switch](examples/switch/main.go) | Switch-case flow chart pattern using `SwitchBlock`. |

---

## Lifecycle hooks

For each transition you can attach optional handlers. Execution order:

1. **BeforeTransition** — Runs before the state change. If it returns an error, the transition is aborted (unless `OnFailure` returns `ErrIgnore`).
2. **Transition** — The main “during” logic. Same abort rules.
3. **State update** — The model’s state is set to the destination state.
4. **AfterTransition** — Runs after the state change. Errors are passed to `OnFailure` if set.
5. **OnSuccess** — Called when the transition completed without error. Its return value is the final model returned to the caller.
6. **OnFailure** — Called when any hook returns an error. If it returns `ErrIgnore`, the error is swallowed and the transition is aborted without changing state.

Handlers that return a non-nil `TransitionModel` pass that model to the next step and as the final result.

### Guard conditions

Use `Guard` to make transitions conditional:

```go
sm.AddTransition(statemachine.Transition{
    Src: []statemachine.State{"ACTIVE"}, Event: "withdraw", Dst: "ACTIVE",
    Guard: func(ctx context.Context, e statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
        acc := m.(*Account)
        evt := e.(WithdrawEvent)
        return acc.Balance >= evt.Amount, nil // Only allow if balance >= amount
    },
})
```

### State entry/exit callbacks

Register callbacks that run when entering or exiting states:

```go
sm.AddStateEntry("IN_PROGRESS", func(ctx context.Context, m statemachine.TransitionModel) error {
    task := m.(*Task)
    task.StartedAt = time.Now().Format(time.RFC3339)
    return nil
})

sm.AddStateExit("IN_PROGRESS", func(ctx context.Context, m statemachine.TransitionModel) error {
    fmt.Println("Leaving IN_PROGRESS state")
    return nil
})
```

### Conditional flow charts

Define multiple transitions from the same (state, event) with different guards. The first matching guard wins:

```go
// High priority -> fast track
sm.AddTransition(statemachine.Transition{
    Src: []statemachine.State{"PENDING"}, Event: "process", Dst: "FAST_TRACK",
    Guard: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
        return m.(*Order).Priority == "high", nil
    },
})

// Medium priority -> standard
sm.AddTransition(statemachine.Transition{
    Src: []statemachine.State{"PENDING"}, Event: "process", Dst: "STANDARD",
    Guard: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
        return m.(*Order).Priority == "medium", nil
    },
})

// Fallback: no guard = always matches if previous guards fail
sm.AddTransition(statemachine.Transition{
    Src: []statemachine.State{"PENDING"}, Event: "process", Dst: "SLOW_LANE",
})
```

### Concurrent execution

Run handlers concurrently for better performance:

```go
// Sequential (default)
sm.AddStateEntry("RUNNING", handler1)
sm.AddStateEntry("RUNNING", handler2)

// Concurrent - run in parallel
sm.AddStateEntryConcurrent("RUNNING", handler3)
sm.AddStateEntryConcurrent("RUNNING", handler4)

// Same for exit handlers
sm.AddStateExitConcurrent("RUNNING", cleanupHandler)
```

**Note**: Concurrent handlers must be thread-safe. Use mutexes if handlers modify shared state.

### If-else blocks (flow chart pattern)

Use `AddConditionBlock` for if-else flow chart patterns:

```go
sm.AddConditionBlock(statemachine.ConditionBlock{
    Src:   []statemachine.State{"PENDING"},
    Event: "process",
    Cases: []statemachine.ConditionCase{
        // if amount >= 10000
        {
            Condition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
                return m.(*Order).Amount >= 10000, nil
            },
            Transition: statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "PREMIUM"},
        },
        // else if amount >= 1000
        {
            Condition: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
                return m.(*Order).Amount >= 1000, nil
            },
            Transition: statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "STANDARD"},
        },
    },
    // else
    ElseTransition: &statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "BASIC"},
})
```

Conditions are evaluated in order; first match executes. `ElseTransition` executes if all conditions fail.

### Switch blocks (flow chart pattern)

Use `AddSwitchBlock` for switch-case flow chart patterns:

```go
sm.AddSwitchBlock(statemachine.SwitchBlock{
    Src:   []statemachine.State{"PENDING"},
    Event: "process",
    SwitchExpr: func(ctx context.Context, _ statemachine.TransitionEvent, m statemachine.TransitionModel) (interface{}, error) {
        return m.(*Payment).Method, nil // Extract value to switch on
    },
    Cases: []statemachine.SwitchCase{
        {Value: "credit_card", Transition: statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "PROCESSING_CARD"}},
        {Value: "paypal", Transition: statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "PROCESSING_PAYPAL"}},
        {Value: "bank_transfer", Transition: statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "AWAITING_TRANSFER"}},
    },
    DefaultTransition: &statemachine.Transition{Src: []statemachine.State{"PENDING"}, Dst: "UNSUPPORTED"},
})
```

Cases are evaluated in order; first match executes. `DefaultTransition` executes if no case matches.

**Priority**: Switch blocks > Condition blocks > Regular transitions

### Parallel transitions

Trigger multiple transitions concurrently:

```go
events := []statemachine.TransitionEvent{event1, event2, event3}
model, err := sm.TriggerParallelTransitions(ctx, events, model)
// Returns first successful result or first error
```

---

## Error handling

The library uses **sentinel errors** so you can branch with `errors.Is`:

| Error | When |
|-------|------|
| `ErrNilModel` | `TriggerTransition` was called with a nil model. |
| `ErrUndefinedTransition` | No transition defined for the model’s current state and the given event. |
| `ErrUninitializedSM` | `AddTransition` / `AddTransitions` on an uninitialized machine (internal). |
| `ErrDuplicateTransition` | A transition for the same (source state, event) was already added. |
| `ErrIgnore` | Special: return this from `OnFailure` to swallow the error and abort the transition without changing state. |

Example:

```go
_, err := sm.TriggerTransition(ctx, ev, model)
if err != nil {
	if errors.Is(err, statemachine.ErrUndefinedTransition) {
		// No transition for this state + event
		return
	}
	if errors.Is(err, statemachine.ErrNilModel) {
		// Model was nil
		return
	}
	return err
}
```

---

## Configuration

### Custom or no-op logger

By default, each transition is logged with the standard `log` package. To disable logging or use your own:

```go
// No logging
sm := statemachine.NewStatemachineWithOptions(statemachine.EventKey{Src: "A", Event: "e"},
	statemachine.WithLogger(statemachine.NoopLogger{}),
)

// Custom logger (e.g. structured logging)
sm := statemachine.NewStatemachineWithOptions(statemachine.EventKey{Src: "A", Event: "e"},
	statemachine.WithLogger(myLogger),
)
```

Your type must implement:

```go
type Logger interface {
	LogTransition(tr Transition)
}
```

### Visualizing the state machine

Call `Visualize` to print a text diagram of all transitions to stdout:

```go
statemachine.Visualize(sm)
```

Safe to call with a nil or empty machine (it prints a message and returns).

---

## API overview

| Symbol | Description |
|--------|-------------|
| `NewStatemachine(startEvent EventKey) StateMachine` | Build a new state machine with default options. |
| `NewStatemachineWithOptions(startEvent EventKey, opts ...Option) StateMachine` | Build with options (e.g. `WithLogger`). |
| `AddTransition(Transition) error` | Register one transition (may have multiple source states). |
| `AddTransitions(...Transition) error` | Register multiple transitions. |
| `TriggerTransition(ctx, event, model) (TransitionModel, error)` | Run the transition for the model’s current state and the given event. Returns the (possibly updated) model or an error. |
| `GetTransitions() (EventKey, map[EventKey]Transition)` | Returns the initial event key and a **copy** of the transition map. |
| `Visualize(StateMachine)` | Print a text diagram of the machine. |

Interfaces:

- **TransitionModel** — `GetState() State`, `SetState(State)`
- **TransitionEvent** — `GetEvent() Event`

---

## Testing

Run tests:

```bash
go test ./statemachine/...
```

Tests cover: valid and invalid transitions, nil model, undefined transition, `ErrIgnore` abort behavior, handler return values, `GetTransitions` copy semantics, and `Visualize` with nil/empty machine.

---

## License

This project is licensed under the MIT License—see [LICENSE](LICENSE) for details.

---

[![BuyMeACoffee](https://bmc-cdn.nyc3.digitaloceanspaces.com/BMC-button-images/custom_images/orange_img.png)](https://www.buymeacoffee.com/imadarsh)
