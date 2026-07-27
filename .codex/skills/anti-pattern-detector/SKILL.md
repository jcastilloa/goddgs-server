---
name: "anti-pattern-detector"
description: "Detect Go design and implementation anti-patterns and turn them into concrete risks and incremental remedies. Use for recurring structural problems involving packages, interfaces, errors, concurrency, initialization, or APIs."
---
# anti-pattern-detector

Detect systemic Go design problems, not isolated style preferences.

## Procedure
1. Inspect the relevant packages, callers, tests, and ownership of goroutines or resources.
2. Record evidence and consequence before naming an anti-pattern.
3. Check for Go-specific cases: interfaces defined by providers, needless abstraction, package-level service locators, work hidden in `init`, ignored or opaque errors, `panic` in normal flows, context misuse, leaking goroutines, unbounded concurrency, and channels used where synchronous code is clearer.
4. Propose the smallest change that restores explicit ownership, simple package boundaries, or predictable control flow.
5. State compatibility and validation needs: public API, error behavior, cancellation, `go test ./...`, and `go test -race ./...` when concurrency is involved.

## Guardrails
- Do not classify idiomatic Go simplicity or deliberate repetition as under-engineering.
- Do not introduce interfaces, factories, layers, channels, or goroutines without a demonstrated need.
- Distinguish a local smell from a repeated architectural anti-pattern.
- Prefer migration steps over rewrites.

## Output
For each finding provide: evidence, risk, proposed remedy, scope, confidence, and validation. Recommend `dependency-simplifier`, `global-state-risk-reviewer`, or `refactoring-candidate-ranker` when useful.
