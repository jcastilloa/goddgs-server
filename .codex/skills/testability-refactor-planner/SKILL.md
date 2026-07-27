---
name: "testability-refactor-planner"
description: "Plan behavior-preserving Go refactors that improve isolation, determinism, and test seams. Use when tests require real I/O, global mutation, sleeps, ordering, broad mocks, or uncontrolled goroutines."
---
# testability-refactor-planner

Improve testability through clearer design, without designing production code around mocks.

## Procedure
1. Identify the behavior to preserve and the obstacle: hidden I/O, nondeterminism, global state, construction, concurrency, or an oversized dependency.
2. Add characterization tests when behavior is unclear.
3. Create the smallest seam: plain parameters, function values, `io.Reader`/`io.Writer`, `fs.FS`, `httptest`, or a narrow interface defined by the consumer.
4. Separate pure decisions from I/O and make goroutine ownership, cancellation, and cleanup observable.
5. Plan small commits with focused tests, `go test ./...`, and `go test -race ./...` for concurrent code.

## Guardrails
- Do not add interfaces solely because a mocking tool expects them.
- Prefer fakes or simple stubs over interaction-heavy mocks; assert behavior rather than implementation calls.
- Do not replace sleeps with different timing assumptions; expose synchronization or completion signals.
- Preserve error identity, cancellation semantics, deadlines, resource cleanup, and parallel-test isolation.

## Output
State obstacle, preserved behavior, proposed seam, production changes, tests to add, sequence, risks, and completion criteria. Route structural extraction to `extract-method-advisor`, `extract-class-advisor`, or `dependency-simplifier`.
