# go-statemachine

[![Go Reference](https://pkg.go.dev/badge/github.com/im-adarsh/go-workflow.svg)](https://pkg.go.dev/github.com/im-adarsh/go-statemachine)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-%3E%3D1.21-00ADD8)](go.mod)

A stateless, fluent state machine for Go that borrows Temporal's vocabulary:
**Workflows**, **Executions**, **Signals**, and **Activities**.

```go
// Define the Workflow once — immutable, shareable, zero state.
workflow, _ := workflow.Define().
    From("PENDING").On("approve").To("APPROVED").Activity(sendApprovalEmail).
    From("PENDING").On("reject").To("REJECTED").
    From("APPROVED").On("ship").To("SHIPPED").
    OnEnterParallel("SHIPPED", updateInventory, notifyWarehouse).
    Build()

// Start an Execution per entity.
exec := workflow.NewExecution("PENDING")
exec.Signal(ctx, "approve", myOrder)
fmt.Println(exec.CurrentState()) // "APPROVED"
```

---

## Table of Contents

- [Concept mapping](#concept-mapping)
- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Conditional Routing](#conditional-routing)
  - [If / ElseIf / Else (Conditions)](#if--elseif--else-conditions)
  - [Switch / Case / Default (SwitchExpr)](#switch--case--default-switchexpr)
- [Activities](#activities)
  - [Transition Activities](#transition-activities)
  - [OnEnter / OnExit Activities](#onenter--onexit-activities)
  - [Parallel Activities](#parallel-activities)
- [Execution](#execution)
- [Stateless vs Stateful](#stateless-vs-stateful)
- [Visualization](#visualization)
- [Error Handling](#error-handling)
- [Logging](#logging)
- [Examples](#examples)
- [API Reference](#api-reference)

---

## Concept Mapping

| Temporal | go-statemachine | Description |
|---|---|---|
| Workflow Definition | `Workflow` | Immutable, compiled state graph. Build once, share freely. |
| Workflow Execution | `Execution` | One live instance per entity. Tracks current state. Thread-safe. |
| Signal | `signal` string + `Execution.Signal()` | Event that drives an Execution to the next state. |
| Activity | `Activity` (`func(ctx, payload) error`) | Unit of work: transition actions, OnEnter/OnExit hooks. |
| Condition / Guard | `Condition` (`func(ctx, payload) bool`) | Predicate for if-else routing. |
| — | `SwitchExpr` (`func(ctx, payload) any`) | Extracts the routing key for switch-case routing. |
| Query | `Execution.CurrentState()` | Read-only inspection of the current state. |

The key difference from Temporal: this library is **in-process and stateless**. There
is no server, no persistence, no durability — just a fast, zero-dependency state router.

---

## Features

- **Temporal vocabulary** — Workflows, Executions, Signals, Activities
- **Fluent builder** — transitions read like a specification
- **Stateless core** — `Workflow.Signal` takes a state, returns a state; no side effects
- **Execution wrapper** — optional thread-safe stateful layer for convenience  
- **Conditional routing** — If/ElseIf/Else Conditions and Switch/Case/Default SwitchExprs
- **Activities** — sequential or parallel; any error aborts the transition
- **OnEnter / OnExit** — state lifecycle hooks as ordinary Activities
- **Visualization** — human-readable diagram with `Workflow.Visualize()`
- **Pluggable logger** — implement one interface method
- **Zero dependencies** — standard library only

---

## Installation

```bash
go get github.com/im-adarsh/go-statemachine
```

Requires **Go 1.21+**.

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/im-adarsh/go-statemachine/workflow"
)

func main() {
    // 1. Define the Workflow (do this once at startup).
    workflow, err := workflow.Define().
        From("CREATED").On("submit").To("SUBMITTED").Activity(validateForm).
        From("SUBMITTED").On("approve").To("APPROVED").
        From("SUBMITTED").On("reject").To("REJECTED").
        From("APPROVED").On("ship").To("SHIPPED").
        OnEnter("SHIPPED", sendConfirmation).
        OnExit("CREATED", auditLog).
        Build()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(workflow.Visualize())

    // 2. Start one Execution per entity.
    ctx := context.Background()
    exec := workflow.NewExecution("CREATED")

    exec.Signal(ctx, "submit", myOrder)
    exec.Signal(ctx, "approve", myOrder)
    exec.Signal(ctx, "ship", myOrder)

    fmt.Println(exec.CurrentState()) // "SHIPPED"
}
```

---

## Conditional Routing

### If / ElseIf / Else (Conditions)

A `Condition` is a predicate evaluated in order; the first `true` wins.
`Else` handles the fallback. Without `Else`, `ErrNoConditionMatched` is returned.

```go
isHighValue   := func(_ context.Context, p any) bool { return p.(*Order).Amount > 5000 }
isMediumValue := func(_ context.Context, p any) bool { return p.(*Order).Amount > 1000 }

workflow, _ := workflow.Define().
    From("SUBMITTED").On("review").
        If(isHighValue).To("MANUAL_REVIEW").
        ElseIf(isMediumValue).To("STANDARD_REVIEW").
        Else("AUTO_APPROVED").
    Build()
```

### Switch / Case / Default (SwitchExpr)

A `SwitchExpr` extracts a routing key; `Case` values are compared with `==`.
`Default` handles unmatched values. Without `Default`, `ErrNoConditionMatched` is returned.

```go
paymentMethod := func(_ context.Context, p any) any {
    return p.(*Order).Method
}

workflow, _ := workflow.Define().
    From("APPROVED").On("pay").
        Switch(paymentMethod).
        Case("card",   "CARD_PROCESSING").
        Case("paypal", "PAYPAL_PROCESSING").
        Default("UNSUPPORTED_PAYMENT").
    Build()
```

---

## Activities

An `Activity` is `func(ctx context.Context, payload any) error` — the same signature
for transition activities and state hooks. This mirrors Temporal's Activity concept:
an isolated, retryable unit of work.

### Transition Activities

Run after OnExit and before OnEnter. If any Activity returns an error, the transition
is **aborted** and the state is unchanged.

```go
workflow, _ := workflow.Define().
    From("DRAFT").On("submit").To("REVIEW").
        Activity(validateForm, enrichMetadata, indexDocument).
    Build()
```

Multiple `Activity()` calls on the same transition append to the same sequential list:

```go
.Activity(a).Activity(b, c)   // equivalent to .Activity(a, b, c)
```

### OnEnter / OnExit Activities

Registered per state, called regardless of which Signal triggered the transition.

```go
workflow, _ := workflow.Define().
    From("A").On("go").To("B").
    OnExit("A",  auditExit, releaseResources).
    OnEnter("B", initState, sendNotification).
    Build()
```

**Execution order for every Signal:**

```
OnExit Activities (current state)   ← sequential, in registration order
    → Transition Activities         ← sequential, in registration order
        → OnEnter Activities        ← sequential (or parallel — see below)
```

### Parallel Activities

Activities within a single `OnEnterParallel` / `OnExitParallel` call run concurrently.
All goroutines are awaited; the first error is returned.

```go
workflow, _ := workflow.Define().
    From("APPROVED").On("complete").To("DONE").
    OnEnterParallel("DONE",
        sendEmail,       // ┐
        updateDatabase,  // ├─ run concurrently
        notifyAnalytics, // ┘
    ).
    Build()
```

Mix sequential and parallel groups on the same state — they run in registration order,
one group at a time:

```go
OnEnter("B", setupState).              // sequential first
OnEnterParallel("B", notify, index).   // then these two in parallel
OnEnter("B", finalAudit)               // then this last
```

---

## Execution

`Execution` wraps a `Workflow` and tracks the current state. It is safe for concurrent use.

```go
exec := workflow.NewExecution("PENDING") // start in PENDING

exec.Signal(ctx, "approve", order)       // drive to next state
exec.CurrentState()                       // read current state (Query)
exec.CanReceive("ship")                   // true if signal is valid right now
exec.SetState("PENDING")                  // override state (no Activities run)
```

`Signal` returns an error on unknown signals, failed Activities, or failed hooks.
On error the state is **unchanged** — except for OnEnter failures, where the new
state is retained (the transition already completed) and the error is returned alongside it.

---

## Stateless vs Stateful

| | `Workflow.Signal` | `Execution.Signal` |
|---|---|---|
| State tracking | Caller's responsibility | Internal (`sync.Mutex`) |
| Thread-safe | ✓ (immutable Workflow) | ✓ |
| Use case | Pure function / event sourcing | One object per entity |

```go
// Stateless — pass in and receive state explicitly:
newState, err := workflow.Signal(ctx, currentState, "approve", order)

// Stateful — Execution tracks state internally:
exec := workflow.NewExecution("PENDING")
err  := exec.Signal(ctx, "approve", order)
```

---

## Visualization

```go
fmt.Println(workflow.Visualize())
```

```
Workflow:
  [APPROVED]
    --ship--> SHIPPED
  [PENDING]
    --approve--> APPROVED
    --reject-->  REJECTED
  [SUBMITTED]
    --review--> [MANUAL_REVIEW | STANDARD_REVIEW | AUTO_APPROVED (else)]
```

---

## Error Handling

```go
import "errors"

newState, err := workflow.Signal(ctx, state, signal, payload)
switch {
case errors.Is(err, workflow.ErrUnknownSignal):
    // No transition registered for (state, signal).
case errors.Is(err, workflow.ErrNoConditionMatched):
    // If-else with no match and no Else; switch with no match and no Default.
case err != nil:
    // Activity or hook returned an error — wrapped with context.
}
```

| Sentinel | When |
|---|---|
| `ErrUnknownSignal` | No transition registered for (state, signal) |
| `ErrNoConditionMatched` | Condition chain exhausted with no match and no fallback |

---

## Logging

Implement `Logger` to plug in any logging library:

```go
type Logger interface {
    LogTransition(from, signal, to string)
}
```

Built-in:

| Type | Behaviour |
|---|---|
| `DefaultLogger{}` | Writes to `log.Printf` |
| `NoopLogger{}` | Discards all output |

```go
// Zap
type zapLogger struct{ z *zap.Logger }
func (l *zapLogger) LogTransition(from, signal, to string) {
    l.z.Info("signal", zap.String("from", from),
                        zap.String("signal", signal),
                        zap.String("to", to))
}

workflow, _ := workflow.Define().
    From("A").On("go").To("B").
    WithLogger(&zapLogger{z: logger}).
    Build()
```

---

## Examples

| Directory | What it shows |
|---|---|
| [`examples/basic`](examples/basic/main.go) | Minimal Workflow + Execution + Logger |
| [`examples/order`](examples/order/main.go) | Order Workflow: Conditions, SwitchExpr, sequential/parallel Activities |
| [`examples/flowchart`](examples/flowchart/main.go) | Loan applications: one Workflow, many concurrent Executions |

```bash
go run ./examples/basic
go run ./examples/order
go run ./examples/flowchart
```

---

## API Reference

### `Define()` — Builder

| Method | Returns | Description |
|---|---|---|
| `Define()` | `*Builder` | Start a new Workflow definition |
| `From(states...)` | `*FromBuilder` | Begin a transition from one or more states |
| `OnEnter(state, activities...)` | `*Builder` | Sequential OnEnter Activities |
| `OnEnterParallel(state, activities...)` | `*Builder` | Concurrent OnEnter Activities |
| `OnExit(state, activities...)` | `*Builder` | Sequential OnExit Activities |
| `OnExitParallel(state, activities...)` | `*Builder` | Concurrent OnExit Activities |
| `WithLogger(Logger)` | `*Builder` | Set the transition logger |
| `Build()` | `(*Workflow, error)` | Compile; returns error on duplicate transitions |
| `MustBuild()` | `*Workflow` | Like Build but panics on error |

### `FromBuilder` / `OnBuilder`

| Method | Returns | Description |
|---|---|---|
| `On(signal)` | `*OnBuilder` | Specify the Signal name |
| `To(dst)` | `*SimpleRouteBuilder` | Simple routing — always goes to dst |
| `If(Condition)` | `*IfBuilder` | Start an if-else chain |
| `Switch(SwitchExpr)` | `*SwitchBuilder` | Start a switch-case chain |

### `SimpleRouteBuilder`

| Method | Returns | Description |
|---|---|---|
| `Activity(fns...)` | `*SimpleRouteBuilder` | Attach transition Activities (chainable) |
| `From / OnEnter / OnExit / … / Build` | — | Finalise and continue or compile |

### `IfBuilder` / `BranchBuilder`

| Method | Returns | Description |
|---|---|---|
| `To(dst)` | `*BranchBuilder` | Set destination for current Condition |
| `ElseIf(Condition)` | `*IfBuilder` | Add another Condition branch |
| `Else(dst)` | `*Builder` | Set fallback destination; finalises chain |

### `SwitchBuilder`

| Method | Returns | Description |
|---|---|---|
| `Case(value, dst)` | `*SwitchBuilder` | Add a case; chainable |
| `Default(dst)` | `*Builder` | Set fallback destination; finalises chain |

### `Workflow`

| Method | Description |
|---|---|
| `Signal(ctx, state, signal, payload)` | Stateless: resolve + run Activities; return new state |
| `NewExecution(initialState)` | Create a stateful Execution |
| `AvailableSignals(state)` | Sorted list of signals valid from state |
| `States()` | Sorted list of all states with outgoing transitions |
| `Visualize()` | Human-readable diagram |

### `Execution`

| Method | Description |
|---|---|
| `Signal(ctx, signal, payload)` | Drive to next state; updates internal state |
| `CurrentState()` | Query the current state |
| `CanReceive(signal)` | True if signal is valid from current state |
| `SetState(state)` | Override state (no Activities run) |

---

## License

[MIT](LICENSE) © 2019-2026 Adarsh Kumar
