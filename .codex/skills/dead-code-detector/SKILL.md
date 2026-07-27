---
name: "dead-code-detector"
description: "Find dead, unreachable, obsolete, or effectively unused Go code and assess whether removal is safe. Use for cleanup of exported APIs, implementations, build-tagged code, stale feature paths, or redundant dependencies."
---
# dead-code-detector

Find removable Go code without confusing static non-use with runtime non-use.

## Procedure
1. Start with compiler, test, `go vet`, and available static-analysis evidence; inspect references across the module and workspace.
2. Classify candidates: unreachable branch, unused private declaration, unused exported API, obsolete implementation, redundant dependency, stale build-tag path, or generated code.
3. Check indirect entry points: interface satisfaction, registration and `init`, reflection, templates, plugins, cgo, `go:linkname`, generated files, commands, tests, examples, and external consumers of exported APIs.
4. For each candidate, choose remove, deprecate then remove, retain with reason, or investigate further.
5. Validate affected build tags and platforms when relevant, then run focused tests and `go test ./...`.

## Guardrails
- The compiler already rejects unused local variables and imports; focus on code it cannot prove dead.
- Do not delete generated code directly when its generator is the source of truth.
- Treat an unreferenced exported symbol as uncertain unless the module is known to be closed to external consumers.
- Do not infer deadness from test coverage alone.

## Output
List candidate, evidence, indirect-use risk, decision, confidence, removal scope, and validation. Send mixed cleanup backlogs to `refactoring-candidate-ranker`.
