---
name: "extract-method-advisor"
description: "Plan safe extraction of mixed-responsibility Go functions or methods into cohesive functions. Use for long control flow, repeated blocks, difficult testing seams, or code that mixes orchestration with parsing, validation, policy, or I/O."
---
# extract-method-advisor

Extract Go functions when the new boundary improves naming, reuse, testing, or control flow.

## Procedure
1. Mark coherent stages, invariants, side effects, error boundaries, and data dependencies.
2. Select a region with one purpose and a small, explicit input/output contract.
3. Prefer a package-level function when receiver state is unnecessary; keep a method when behavior belongs to the receiver's invariant.
4. Preserve evaluation order, error identity and wrapping, named-return behavior, `defer` scope, resource lifetime, cancellation, locks, and goroutine ownership.
5. Extract incrementally, format, and run focused tests plus `go test ./...`; use the race detector when synchronization changes.

## Guardrails
- Do not create tiny helpers that merely rename obvious statements or force readers to jump around.
- Do not replace explicit parameters with package state or broad structs to shorten a signature.
- Avoid boolean control flags; prefer distinct operations when behavior differs.
- Watch for loop-variable capture, pointer aliasing, and `defer` timing changes.

## Output
Provide extraction boundary, proposed signature, moved logic, preserved invariants, risks, steps, and validation. Use `testability-refactor-planner` when the main goal is creating test seams.
