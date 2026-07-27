---
name: "refactoring-candidate-ranker"
description: "Rank Go refactoring candidates by evidence, payoff, urgency, effort, and change risk. Use after a review produces several findings or when a team needs a safe, ordered refactoring backlog."
---
# refactoring-candidate-ranker

Prioritize refactors that solve observed problems and can be validated safely.

## Procedure
1. Normalize each candidate into evidence, affected behavior, expected payoff, scope, dependencies, and validation signals.
2. Estimate impact on correctness, maintainability, testability, delivery, public API, package graph, error contracts, and concurrency.
3. Rank by payoff and urgency, adjusted for confidence, effort, blast radius, and test coverage.
4. Prefer enabling changes that reduce risk for later work: characterization tests, explicit dependencies, smaller functions, or removal of dead paths.
5. Order candidates into independent, reversible steps and identify items to defer or reject.

## Guardrails
- Do not prioritize by code size, aesthetic preference, or generic severity alone.
- Treat exported API changes, package moves, serialization changes, error identity, and synchronization changes as higher-risk.
- Do not give false numerical precision; explain close calls briefly.
- Include `go test ./...`, relevant build tags, and `go test -race ./...` where applicable.

## Output
Provide a compact ranked table: candidate, evidence, payoff, urgency, effort, risk, confidence, prerequisite, and next skill. Use `extract-method-advisor`, `extract-class-advisor`, or `testability-refactor-planner` for execution planning.
