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

Each `main.go` is self-contained and includes a short doc comment and run instruction at the top.
