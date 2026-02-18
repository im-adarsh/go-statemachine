# Examples

Runnable examples for **go-statemachine**. From the project root:

```bash
go run ./examples/<name>/main.go
```

| Example | Description |
|--------|-------------|
| [basic](basic/main.go) | Minimal FSM: one transition, no hooks. |
| [phase](phase/main.go) | Phase diagram (SOLID ↔ LIQUID ↔ GAS) with before/during/after hooks and `Visualize`. |
| [error-handling](error-handling/main.go) | Using `errors.Is` for undefined transitions and `ErrIgnore` to abort without changing state. |
| [custom-logger](custom-logger/main.go) | Disable logging with `NoopLogger` or use a custom `Logger` implementation. |
| [guard](guard/main.go) | Conditional transitions using guard conditions (e.g., balance check before withdrawal). |
| [state-callbacks](state-callbacks/main.go) | State entry and exit callbacks for actions when entering/exiting states. |
| [conditional-flow](conditional-flow/main.go) | Multiple transitions from same state+event with guards (conditional flow chart). |
| [concurrent-execution](concurrent-execution/main.go) | Running handlers concurrently for better performance. |
| [if-else](if-else/main.go) | **NEW**: If-else flow chart pattern using `ConditionBlock`. |
| [switch](switch/main.go) | **NEW**: Switch-case flow chart pattern using `SwitchBlock`. |

Each `main.go` is self-contained and includes a short doc comment and run instruction at the top.
