---
name: "dependency-simplifier"
description: "Simplify Go package and component dependencies while preserving behavior and APIs. Use for tangled imports, broad interfaces, hard-to-construct services, excessive fan-out, dependency cycles in the design, or unclear package boundaries."
---
# dependency-simplifier

Reduce coupling while keeping the Go design direct.

## Procedure
1. Map relevant packages, imports, constructors, concrete dependencies, interfaces, side effects, and callers.
2. Identify the costly edge: unstable dependency, wrong ownership, oversized interface, package that mixes policy and I/O, or dependency reached through globals.
3. Move behavior toward the package that owns the data; keep packages cohesive and avoid generic `util`, `common`, or `service` dumping grounds.
4. Prefer concrete dependencies by default. When substitution is required, define the smallest interface at the consuming side; use a function dependency when it is the clearer seam.
5. Introduce changes incrementally and verify import graph, public API, tests, and race behavior.

## Guardrails
- Do not create interfaces only to mirror a concrete type or satisfy a mocking convention.
- Do not split packages by file, layer, or type without a cohesive boundary.
- Avoid dependency containers and service locators; make required dependencies explicit through construction or parameters.
- Preserve `internal` boundaries and avoid new import cycles.

## Output
Show the problematic dependency, its cost, the proposed boundary, migration steps, compatibility impact, and validation. Use `hidden-dependency-detector` first when dependencies are not explicit.
