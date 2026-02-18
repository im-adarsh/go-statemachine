# ShopFlow — go-statemachine Example

A single, end-to-end e-commerce order processing application that demonstrates
**every feature** of go-statemachine in one coherent codebase.

## Run it

```bash
go run ./examples/shopflow
```

## What it covers

| Scenario | Feature demonstrated |
|---|---|
| 1 — Happy Path | Basic transitions · `OnEnterConcurrent` · `ExecutionHooks` · `History` · `Await` |
| 2 — Fraud Rejection | `If / ElseIf / Else` conditional routing |
| 3 — Tier Routing | `Switch / Case / Default` routing |
| 4 — Saga Rollback | `.Saga(activity, compensation)` — automatic reverse compensation |
| 5 — Retry & Timeout | `workflow.Retry(fn, policy)` · `workflow.Timeout(fn, d)` |
| 6 — Middleware | `builder.WithMiddleware()` · `workflow.WithMiddleware()` |
| 7 — Cancellation & Await | `exec.Cancel()` · `exec.Done()` · `exec.Await()` |

## State machine

```
PENDING ──validate──▶ PAYMENT_HOLD ──fulfil──▶ FULFILLING_[TIER] ──ship──▶ SHIPPED ──deliver──▶ DELIVERED
   │         │                                                                   │
   │       REJECTED                                                              │
   │                                                                             │
   └─────────────────────── cancel (any state) ─────────────────────────────────▶ CANCELLED
```

See [`main.go`](shopflow/main.go) for the full source.
