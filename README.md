# go-statemachine

A lightweight, stateless, type-safe state machine for Go — modelled after [Temporal](https://temporal.io/)'s core concepts but with **zero infrastructure**. Define a `Workflow` once, run unlimited `Execution`s anywhere.

[![Go Reference](https://pkg.go.dev/badge/github.com/im-adarsh/go-statemachine/workflow.svg)](https://pkg.go.dev/github.com/im-adarsh/go-statemachine/workflow)
[![Go Report Card](https://goreportcard.com/badge/github.com/im-adarsh/go-statemachine)](https://goreportcard.com/report/github.com/im-adarsh/go-statemachine)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-microsite-58a6ff?logo=github)](https://im-adarsh.github.io/go-statemachine)

**[📖 Read the full documentation & ShopFlow walkthrough →](https://im-adarsh.github.io/go-statemachine)**

---

## Why go-statemachine?

| Feature | go-statemachine | bare `switch` | full Temporal |
|---|---|---|---|
| Type-safe payload | ✅ generics | ❌ | ✅ |
| Fluent builder API | ✅ | ❌ | ❌ |
| Conditional / Switch routing | ✅ | manual | ✅ |
| Saga compensation | ✅ | ❌ | ✅ |
| Activity retry + timeout | ✅ | ❌ | ✅ |
| Middleware / interceptors | ✅ | ❌ | ✅ |
| Parallel state hooks | ✅ | ❌ | ✅ |
| Execution history | ✅ | ❌ | ✅ |
| Await (condition blocking) | ✅ | ❌ | ✅ |
| Execution cancellation | ✅ | ❌ | ✅ |
| Infrastructure required | ❌ none | ❌ none | ✅ server |

---

## Installation

```bash
go get github.com/im-adarsh/go-statemachine/workflow
```

Requires **Go 1.21+** (generics).

---

## Core Concepts (Temporal-inspired)

| go-statemachine | Temporal equivalent | Description |
|---|---|---|
| `Workflow[T]` | Workflow Definition | Immutable state graph; share across goroutines |
| `Execution[T]` | Workflow Execution | Stateful instance for one entity |
| `Signal` | Signal | Named event that drives a transition |
| `Activity[T]` | Activity Function | Unit of work; used for transitions and hooks |
| `Condition[T]` | — | Predicate for if-else routing |
| `SwitchExpr[T]` | — | Expression for switch-case routing |
| `Middleware[T]` | Activity Interceptor | Wraps activities for cross-cutting concerns |

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/im-adarsh/go-statemachine/workflow"
)

type Order struct{ ID string; Approved bool }

var orderWF = workflow.Define[*Order]().
    From("PENDING").On("approve").To("APPROVED").
    From("PENDING").On("reject").To("REJECTED").
    From("APPROVED").On("ship").To("SHIPPED").
    MustBuild()

func main() {
    ctx := context.Background()
    exec := orderWF.NewExecution(ctx, "PENDING")

    o := &Order{ID: "ORD-1", Approved: true}
    exec.Signal(ctx, "approve", o)
    exec.Signal(ctx, "ship", o)

    fmt.Println(exec.CurrentState()) // SHIPPED
}
```

---

## Features

### Type-safe generics

No more `payload.(MyType)` — the payload type is baked into the workflow:

```go
wf := workflow.Define[*Order]().
    From("PENDING").On("approve").To("APPROVED").
    Activity(func(ctx context.Context, o *Order) error {
        // o is *Order — no type assertion needed
        return chargeCard(o.Amount)
    }).
    MustBuild()
```

---

### Conditional Routing — if/else

```go
workflow.Define[*Order]().
    From("PENDING").On("review").
    If(func(_ context.Context, o *Order) bool { return o.Approved }, "APPROVED").
    ElseIf(func(_ context.Context, o *Order) bool { return o.Amount < 10 }, "AUTO_APPROVED").
    Else("REJECTED")
```

### Conditional Routing — switch

```go
workflow.Define[*Order]().
    From("PROCESSING").On("route").
    Switch(func(_ context.Context, o *Order) any { return o.Tier }).
    Case("premium", "VIP_FULFIL").
    Case("standard", "STANDARD_FULFIL").
    Default("STANDARD_FULFIL")
```

If no branch matches and no `Else`/`Default` is defined, `Signal` returns `ErrNoConditionMatched`.

---

### Transition Activities

Activities run **sequentially** in the order registered, after OnExit hooks and before OnEnter hooks:

```go
From("A").On("go").To("B").
    Activity(validateOrder, reserveInventory, sendConfirmationEmail)
```

---

### Saga Compensation

Register activities as Saga steps. If a later step fails, completed steps are compensated in reverse order:

```go
From("PROCESSING").On("fulfil").To("FULFILLED").
    Saga(chargeCard, refundCard).           // step 1: charge; comp: refund
    Saga(deductInventory, restoreInventory). // step 2: deduct; comp: restore
    Activity(scheduleShipping)              // step 3: no compensation needed
```

If `scheduleShipping` fails → `restoreInventory` then `refundCard` are called automatically.

---

### Retry with Backoff

```go
var policy = workflow.RetryPolicy{
    MaxAttempts:        3,
    InitialInterval:    500 * time.Millisecond,
    BackoffCoefficient: 2.0,                    // 500ms → 1s → 2s
    MaxInterval:        10 * time.Second,
    NonRetryableErrors: []error{ErrInvalidCard}, // abort immediately on these
}

.Activity(workflow.Retry(chargeCard, policy))
```

---

### Activity Timeout

```go
.Activity(workflow.Timeout(callThirdPartyAPI, 5*time.Second))
```

---

### Middleware

**Per-activity** — compose before registering:

```go
.Activity(workflow.WithMiddleware(chargeCard, otelMiddleware, metricsMiddleware))
```

**Workflow-wide** — applied to every activity and hook:

```go
workflow.Define[*Order]().
    WithMiddleware(otelMiddleware, metricsMiddleware).
    ...
```

Middleware type:

```go
type Middleware[T any] func(Activity[T]) Activity[T]
```

---

### State Hooks

Run activities when entering or leaving a state:

```go
// Sequential (default)
workflow.Define[*Order]().
    OnEnter("APPROVED", sendApprovalEmail).
    OnExit("PENDING", releaseReservation)

// Concurrent — all activities in the group run in parallel goroutines
workflow.Define[*Order]().
    OnEnterConcurrent("SCREENING", creditCheck, fraudDetection)
```

---

### Execution Lifecycle Hooks

```go
exec := wf.NewExecution(ctx, "PENDING",
    workflow.WithHooks(workflow.ExecutionHooks[*Order]{
        OnTransition: func(ctx context.Context, from, to, signal string, o *Order) {
            log.Printf("[%s] %s --%s--> %s", o.ID, from, signal, to)
        },
        OnError: func(ctx context.Context, state, signal string, err error, o *Order) {
            metrics.Inc("workflow.errors", "state", state)
        },
    }),
)
```

---

### Execution History

Every `Signal` call is recorded — success or failure:

```go
for _, e := range exec.History() {
    fmt.Printf("%s --%s--> %s  took=%v  err=%v\n",
        e.FromState, e.Signal, e.ToState, e.Duration, e.Err)
}
```

---

### Cancellation

```go
exec := wf.NewExecution(ctx, "PENDING")

// Cancel interrupts in-flight activities that respect ctx.Done()
// and prevents future Signals from running.
exec.Cancel()

// Done returns a channel closed when the Execution is cancelled.
<-exec.Done()

// Subsequent signals return ErrExecutionCancelled.
err := exec.Signal(ctx, "approve", o) // errors.Is(err, workflow.ErrExecutionCancelled)
```

---

### Await

Block until a state condition is met, the Execution is cancelled, or a timeout fires:

```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()

err := exec.Await(ctx, func(state string) bool {
    return state == "SHIPPED"
})
```

---

### Stateless core

`Workflow.Signal` is a pure function — no internal state:

```go
// Use Execution for stateful single-entity tracking (common case):
exec := wf.NewExecution(ctx, "PENDING")
exec.Signal(ctx, "approve", order)

// Or drive the state machine yourself (e.g. state stored in a DB):
newState, err := wf.Signal(ctx, currentStateFromDB, "approve", order)
// persist newState...
```

---

## Workflow Visualisation

```go
fmt.Println(wf.Visualize())
```

```
Workflow:
  [APPROVED]
    --ship--> SHIPPED
  [PENDING]
    --approve--> APPROVED
    --reject--> REJECTED
```

---

## API Reference

### `Define[T]()`

```go
func Define[T any]() *Builder[T]
```

Entry point for the fluent builder.

### Builder methods

| Method | Description |
|---|---|
| `From(states...)` | Starts a transition definition |
| `OnEnter(state, activities...)` | Sequential on-enter hooks |
| `OnEnterConcurrent(state, activities...)` | Parallel on-enter hooks |
| `OnExit(state, activities...)` | Sequential on-exit hooks |
| `OnExitConcurrent(state, activities...)` | Parallel on-exit hooks |
| `WithLogger(Logger)` | Set custom logger |
| `WithMiddleware(mw...)` | Workflow-wide activity middleware |
| `Build()` | Compile; returns `(*Workflow[T], error)` |
| `MustBuild()` | Compile; panics on error |

### Route builder methods (chained after `From(...).On(...)`)

| Method | Description |
|---|---|
| `.To(dst)` | Simple unconditional route → `*SimpleRouteBuilder` |
| `.If(cond, dst)` | Conditional route → `*IfBuilder` |
| `.Switch(expr)` | Switch route → `*SwitchBuilder` |

### `*SimpleRouteBuilder` methods

| Method | Description |
|---|---|
| `.Activity(fns...)` | Register transition activities |
| `.Saga(activity, compensate)` | Register a Saga step with compensation |

### `Workflow[T]` methods

| Method | Description |
|---|---|
| `Signal(ctx, state, signal, payload)` | Stateless transition; returns new state |
| `NewExecution(ctx, initialState, opts...)` | Create a stateful Execution |
| `AvailableSignals(state)` | Signals accepted in state |
| `States()` | All states with outgoing transitions |
| `Visualize()` | Text diagram |

### `Execution[T]` methods

| Method | Description |
|---|---|
| `Signal(ctx, signal, payload)` | Drive transition; thread-safe |
| `CurrentState()` | Current state |
| `CanReceive(signal)` | Signal accepted in current state? |
| `SetState(state)` | Forcibly set state (no hooks/activities) |
| `AvailableSignals()` | Signals accepted now |
| `History()` | Snapshot of all recorded transitions |
| `Cancel()` | Cancel the Execution |
| `Done()` | Channel closed on Cancel |
| `Await(ctx, func(state string) bool)` | Block until condition or cancellation |

### Activity wrappers

| Function | Description |
|---|---|
| `Retry(fn, RetryPolicy)` | Exponential-backoff retry |
| `Timeout(fn, duration)` | Per-call deadline |
| `WithMiddleware(fn, mw...)` | Compose middleware around one activity |

### Errors

| Sentinel | Meaning |
|---|---|
| `ErrUnknownSignal` | No transition registered for signal in current state |
| `ErrNoConditionMatched` | Conditional route had no matching branch |
| `ErrRetryExhausted` | All retry attempts failed |
| `ErrExecutionCancelled` | Execution was cancelled |

---

## Examples

| Example | What it shows |
|---|---|
| [`examples/basic`](examples/basic/main.go) | Traffic light, history |
| [`examples/order`](examples/order/main.go) | Saga, retry, timeout, Await, conditional routing |
| [`examples/flowchart`](examples/flowchart/main.go) | Concurrent executions, parallel hooks, middleware |

---

## License

MIT — see [LICENSE](LICENSE).
