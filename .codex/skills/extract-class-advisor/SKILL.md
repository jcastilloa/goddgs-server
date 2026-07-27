---
name: "extract-class-advisor"
description: "Split overloaded Go structs, types, or packages into cohesive collaborators. Use when a type has unrelated fields or method groups, a package owns several responsibilities, or changes repeatedly affect only distinct subsets of state."
---
# extract-class-advisor

Apply the intent of Extract Class idiomatically in Go: extract a cohesive type, component, or package only when the boundary is real.

## Procedure
1. Group fields, methods, functions, dependencies, and invariants by responsibility and change pattern.
2. Identify ownership of mutable state, resources, locks, goroutines, and lifecycle.
3. Choose the smallest boundary: package-local helper type, composed collaborator, standalone function, or separate package when reuse and dependency direction justify it.
4. Keep interfaces at consumers and only where multiple behavior sources or testing seams require substitution.
5. Migrate callers incrementally; preserve zero-value expectations, method sets, embedding behavior, public API, and concurrency guarantees.

## Guardrails
- Go has no classes; do not translate class-centric patterns into one-type packages or getter/setter shells.
- Do not split data that must remain atomic under the same mutex or invariant.
- Prefer composition, but avoid forwarding layers with no independent responsibility.
- Avoid a new package if the extracted concept is only an implementation detail.

## Output
Describe current responsibilities, proposed ownership, moved state and behavior, API impact, migration sequence, and tests. Use `dependency-simplifier` if package direction is the primary problem.
