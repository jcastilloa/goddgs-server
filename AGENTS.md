# AGENTS.md — goddgs-server API

Go API template with layered architecture, versioned routes, and handlers grouped by category.

## Mandatory Skills

Before making changes in this repository, load and apply these local skills:

- `.codex/skills/clean-code`
- `.codex/skills/go-clean-ddd-hexagonal`
- `.codex/skills/go-refactoring`
- `.codex/skills/go-code-simplification`
- `.codex/skills/golang-pro`
- `.codex/skills/sql-pro`
- `.codex/skills/tdd-workflows-tdd-cycle`
- `.codex/skills/tdd-workflows-tdd-red`
- `.codex/skills/tdd-workflows-tdd-green`
- `.codex/skills/tdd-workflows-tdd-refactor`

Notes:

- The scaffold reads these skills from `template/skills/` in this repository and copies them into `.codex/skills/` in the generated project.
- `.claude/skills` and `.opencode/skills` are symlinks to `.codex/skills` in the generated project.

## Structure

```text
goddgs-server/
├── cmd/api/main.go
├── platform/
│   ├── config/
│   ├── di/
│   ├── server/
│   ├── routes/
│   ├── handlers/
│   │   └── search/
│   └── proxy/
│       └── ssh/
├── search/
│   ├── application/
│   └── domain/
├── proxy/
│   └── application/
└── shared/
    └── config/domain/
```

## HTTP Convention

- HTTP server wiring lives in `platform/server/server.go`.
- Routes are registered by category in `platform/routes/*.go`.
- Handlers are grouped by category in `platform/handlers/<category>/`.
- Default version prefix: `/v1`.

Included examples:

- `GET /v1/hello`
- `GET /v1/version`

## Adding a New Handler

1. Create the handler in `platform/handlers/<category>/<action>.go` implementing `handlers.Handler`.
2. Add a constant label for DI registration.
3. Register it in `platform/di/container.go`.
4. Add/update the route in `platform/routes/<category>.go`.
5. Register that route group in `server.registerRoutes()`.

## Layer Dependencies

```text
platform -> search + proxy + shared
cmd -> platform + search + proxy + shared
```

Do not allow `shared`, `search/domain`, `search/application`, or `proxy/application` to import `platform`.

## Service and Repository Size Rule

- Do not create giant monolithic services or repositories.
- Split responsibilities into multiple focused `.go` files, following `clean-code` (SRP, small units, clear naming).
- Prefer composition of small components over one large file/class handling everything.

## First-Level Business Entities

Specific entities (example: `post`) live at the project root:

```text
post/
├── application/
└── domain/
```

- `domain`: pure business rules, no infrastructure imports.
- `application`: use cases.
- technical adapters: `platform/<tech>/post/...`.

## Checklist

- [ ] New routes are under the versioned prefix.
- [ ] New handlers are registered in DI.
- [ ] No `platform` imports inside `shared` or `domain`.
- [ ] Configuration is read through `viper` (`config.yaml`/env).
- [ ] `go test ./...` and `go vet ./...` pass.
