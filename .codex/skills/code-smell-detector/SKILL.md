---
name: "code-smell-detector"
description: "Identify actionable code smells in Go code, diffs, functions, types, or packages. Use for a broad first pass when maintainability, correctness, testability, API clarity, or concurrency safety is in doubt."
---
# code-smell-detector

Find evidence-backed Go smells and explain their cost.

## Procedure
1. Inspect the target plus its callers, tests, package boundary, and error or concurrency paths.
2. Look for mixed-responsibility functions, oversized structs or interfaces, primitive or boolean-heavy APIs, feature envy across packages, shotgun changes, duplication, hidden dependencies, mutable globals, unclear ownership, and error handling that loses context or identity.
3. Treat goroutines without lifecycle control, unclear channel ownership, missing cancellation, copied mutexes, and unsafe shared state as correctness risks, not merely style issues.
4. Check whether a proposed cleanup would make Go code less direct through premature interfaces, packages, generics, reflection, or concurrency.
5. Rank only findings with a concrete maintenance, defect, or testing consequence.

## Guardrails
- Do not report formatting or naming that `gofmt`, `go vet`, or an established linter already handles unless it exposes a design issue.
- Do not flag every long function, exported symbol, package variable, or repeated block without contextual harm.
- Separate observed evidence from inference and note missing runtime context.

## Output
List location, smell, evidence, consequence, smallest safe refactor, confidence, and validation. Route focused work to `duplication-reviewer`, `dead-code-detector`, `hidden-dependency-detector`, or `refactoring-candidate-ranker`.
