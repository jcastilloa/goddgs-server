---
name: "hidden-dependency-detector"
description: "Detect ambient and implicit dependencies in Go that make behavior unpredictable or hard to test. Use for direct access to time, randomness, environment, filesystem, network, globals, `init` registration, context values, or unmanaged goroutines."
---
# hidden-dependency-detector

Reveal dependencies that are absent from a function, method, or constructor contract.

## Procedure
1. Trace I/O and nondeterminism: package variables, `time.Now`, timers, randomness, environment, working directory, filesystem, network, default clients, process signals, `init`, registries, and background goroutines.
2. Record where each dependency is selected, configured, owned, and stopped.
3. Separate legitimate process concerns from domain logic that requires a controllable seam.
4. Make dependencies explicit with the lightest Go mechanism: parameter, constructor field, function value, `io.Reader`/`io.Writer`, `fs.FS`, or a small consumer-defined interface.
5. Keep `context.Context` for cancellation, deadlines, and request-scoped values; do not use it as a general dependency bag.

## Guardrails
- Do not inject every standard-library function; extract only seams needed for determinism, substitution, or ownership.
- Avoid broad environment or clock interfaces when one value or function suffices.
- Do not hide dependency creation inside a new global accessor.
- Include cleanup and goroutine termination in every lifecycle recommendation.

## Output
List hidden dependency, evidence, effect, proposed contract, ownership, scope, and validation. Route mutable globals to `global-state-risk-reviewer` and boundary redesign to `dependency-simplifier`.
