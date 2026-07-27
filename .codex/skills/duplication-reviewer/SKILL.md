---
name: "duplication-reviewer"
description: "Review structural, logical, and semantic duplication in Go and choose whether to consolidate it. Use when repeated functions, handlers, validation, data transformations, or parallel types create inconsistent changes or defects."
---
# duplication-reviewer

Consolidate knowledge, not merely similar syntax.

## Procedure
1. Group duplicates by shared behavior and reason to change, not token similarity.
2. Compare inputs, outputs, errors, side effects, ordering, allocation, concurrency, and package ownership.
3. Choose among leaving repetition, extracting a function or type, sharing a package-local helper, moving behavior to its owner, or using generics when the algorithm is genuinely type-independent.
4. Keep the abstraction narrower than the duplicated cases and preserve error identity and observable behavior.
5. Validate all former copies with table-driven tests or shared behavioral tests where useful.

## Guardrails
- A little repetition is often clearer than reflection, `any`, callback-heavy helpers, or premature generics.
- Do not couple unrelated packages merely because their implementations look alike.
- Do not hide meaningful domain differences behind flags or oversized parameter objects.
- Account for typed-nil, zero-value, aliasing, and pointer/value differences when consolidating.

## Output
For each group provide locations, shared knowledge, important variation, decision, target abstraction, risk, and validation. Route local extraction to `extract-method-advisor` or responsibility splits to `extract-class-advisor`.
