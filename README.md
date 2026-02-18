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
