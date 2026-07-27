---
name: "global-state-risk-reviewer"
description: "Assess mutable package-level state and singleton-like dependencies in Go. Use for package variables, global clients, registries, caches, mutable defaults, `init` side effects, or tests and goroutines that interfere with one another."
---
# global-state-risk-reviewer

Review shared state according to mutability, ownership, lifecycle, and concurrency.

## Procedure
1. Inventory package variables, registries, caches, default clients, `sync.Once`, `init` mutations, environment-derived state, and test overrides.
2. Trace writers, readers, initialization order, reset behavior, goroutines, and synchronization.
3. Classify each item as immutable configuration, concurrency-safe shared service, scoped state, or unsafe ambient state.
4. Prefer explicit construction and ownership. Use parameters, small consumer-side interfaces, or function dependencies; retain process-wide state only with a clear lifecycle and synchronization contract.
5. Validate parallel tests, shutdown, repeated initialization, and `go test -race ./...`.

## Guardrails
- Do not flag constants, immutable lookup tables, or concurrency-safe stateless values merely for being package-level.
- Dependency injection does not require a framework or an interface for every concrete type.
- Do not replace visible globals with a hidden service locator or mutable context values.
- Preserve initialization errors instead of moving them into `init` or `Must` helpers without justification.

## Output
List state, readers/writers, lifecycle, race or test risk, recommendation, migration, and validation. Use `hidden-dependency-detector` to expand the ambient-dependency audit.
