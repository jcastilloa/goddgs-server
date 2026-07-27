---
name: "go-refactoring"
description: "Route Go code-smell and refactoring work to the smallest relevant skill or sequence of skills. Use as the master entry point when the needed analysis is unclear, several refactoring concerns overlap, or an end-to-end review and plan is requested."
---
# Go refactoring skills index

Select only the skills needed. Start broad only when the problem is not yet classified; do not run the full set by default.

## Skill selection

| Skill | Purpose | Use when |
|---|---|---|
| [code-smell-detector](../code-smell-detector/SKILL.md) | Broad evidence-based smell scan | The code feels costly or risky but the cause is unclear |
| [anti-pattern-detector](../anti-pattern-detector/SKILL.md) | Recurring design and implementation failures | A structural problem repeats across types, packages, APIs, or concurrency flows |
| [dead-code-detector](../dead-code-detector/SKILL.md) | Safe identification and removal of obsolete Go code | Suspected unused exports, implementations, build-tag paths, or dependencies need proof |
| [dependency-simplifier](../dependency-simplifier/SKILL.md) | Simplify packages and explicit dependencies | Imports, interfaces, construction, or package boundaries are tangled |
| [duplication-reviewer](../duplication-reviewer/SKILL.md) | Decide whether and how to consolidate repetition | Similar logic changes together or repeatedly diverges |
| [extract-method-advisor](../extract-method-advisor/SKILL.md) | Extract cohesive Go functions | A function mixes orchestration, policy, validation, or I/O |
| [extract-class-advisor](../extract-class-advisor/SKILL.md) | Split overloaded structs, types, or packages | Unrelated state and behavior are owned together |
| [global-state-risk-reviewer](../global-state-risk-reviewer/SKILL.md) | Review mutable process-wide state | Package variables, singletons, registries, or tests share mutable state |
| [hidden-dependency-detector](../hidden-dependency-detector/SKILL.md) | Reveal ambient I/O and nondeterminism | Time, randomness, environment, context values, globals, or goroutines are implicit |
| [refactoring-candidate-ranker](../refactoring-candidate-ranker/SKILL.md) | Order a refactoring backlog | Several valid findings compete for attention |
| [testability-refactor-planner](../testability-refactor-planner/SKILL.md) | Design safe test seams | Tests need real I/O, sleeps, globals, broad mocks, or uncontrolled concurrency |

## Routing

- Unknown problem: `code-smell-detector` → focused skill → `refactoring-candidate-ranker` if several findings remain.
- Architecture or recurring failure: `anti-pattern-detector` → `dependency-simplifier` or state/dependency review.
- Hard-to-test code: `hidden-dependency-detector` → `testability-refactor-planner`.
- Large function or owner: `extract-method-advisor` or `extract-class-advisor`; use both only when both boundaries are independently justified.
- Cleanup: `dead-code-detector` and `duplication-reviewer`, then rank changes when risk or scope differs.

## Master rules

1. Inspect Go code, callers, tests, package boundaries, error behavior, and concurrency lifecycle before recommending change.
2. Prefer the smallest idiomatic, behavior-preserving refactor; do not add interfaces, packages, generics, reflection, channels, or goroutines without evidence.
3. Separate facts from inference and state compatibility risks involving exported APIs, errors, serialization, build tags, platforms, and synchronization.
4. Validate incrementally with formatting, focused tests, `go test ./...`, and `go test -race ./...` when concurrency is affected.
5. Return findings with evidence, consequence, recommendation, confidence, and validation; name the next skill only when another pass adds value.
