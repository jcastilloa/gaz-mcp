# AGENTS.md — gaz-mcp MCP

Read-only MySQL + PostgreSQL MCP proxy. Hexagonal architecture, `cobra` + `viper`, DI.

## Mandatory Skills

Before making changes in this repository, load and apply these local skills:

- `.codex/skills/gaz-mcp-db`
- `.codex/skills/clean-code`
- `.codex/skills/golang-pro`
- `.codex/skills/sql-pro`
- `.codex/skills/tdd-workflows-tdd-cycle`
- `.codex/skills/tdd-workflows-tdd-red`
- `.codex/skills/tdd-workflows-tdd-green`
- `.codex/skills/tdd-workflows-tdd-refactor`

Notes:

- `.claude/skills` and `.opencode/skills` are symlinks to `.codex/skills`.
- `gaz-mcp-db` documents the MCP tool interface and usage patterns for the SQL proxy.

## Structure

```text
gaz-mcp/
├── cmd/server/main.go
├── mcp/
│   ├── application/sql/
│   └── domain/sql/
├── platform/
│   ├── config/
│   ├── di/
│   ├── mcp/
│   │   ├── commands/
│   │   ├── server/
│   │   ├── sql/
│   │   └── tools/
│   └── openai/
└── shared/
    ├── ai/domain/
    └── config/domain/
```

## MCP Tools

| Tool        | Parameters                        | Description                                               |
|-------------|-----------------------------------|-----------------------------------------------------------|
| `sql_query` | `environment`, `database`, `query` | Read-only SQL across environments (MySQL + PostgreSQL) |

## MCP Convention

- Entry point: `cmd/server/main.go`.
- CLI flags managed with `cobra` and bound to `viper`.
- MCP server wiring in `platform/mcp/server/server.go`.
- Tool definitions in `platform/mcp/tools/`, registered in `platform/mcp/commands/root.go`.
- Default transport: `stdio`.

## Adding a New MCP Tool

1. Create a domain port under `mcp/domain/<feature>/` if business logic is needed.
2. Add the use case in `mcp/application/<feature>/`.
3. Implement infrastructure adapters in `platform/mcp/<feature>/`.
4. Create the tool definition and handler in `platform/mcp/tools/<feature>.go`.
5. Register dependencies in `platform/di/container.go`.
6. Register the tool in `platform/mcp/commands/root.go`.

## Layer Dependencies

```text
platform -> shared + mcp/application + mcp/domain
mcp/application -> mcp/domain
cmd -> platform + shared
```

Do not allow `shared` or `mcp/domain` to import `platform`.

## Configuration

MySQL connection configured via Viper (see `config.sample.yaml`):

```yaml
mysql:
  host: 127.0.0.1
  port: 3306
  user: root
  password: changeme
```

The `database` is selected dynamically per-query via the tool parameter — not from config.

## Checklist

- [ ] New tools are registered in startup wiring.
- [ ] Flags are bound via `viper` and support env overrides.
- [ ] No `platform` imports inside `shared` or `mcp/domain`.
- [ ] `go build ./...` and `go vet ./...` pass.
