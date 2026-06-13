# gaz-mcp

Read-only MySQL and PostgreSQL proxy exposed as an MCP server. Lets AI coding agents explore schemas and run `SELECT` queries safely — no writes, no accidental data loss.

## Overview

`gaz-mcp` wraps MySQL and PostgreSQL connections in an MCP-compatible stdio server. AI agents call the `sql_query` tool to inspect tables, describe columns, and query data across both engines.

Every query runs inside a **read-only database transaction** and is validated at the application layer. Write statements (`INSERT`, `UPDATE`, `DELETE`, `DROP`, etc.) are rejected before they reach the database.

## Features

- **MCP stdio server** — one binary, no daemon, no network ports.
- **MySQL + PostgreSQL** — dual-engine support, selectable per query via `type` parameter.
- **Read-only enforcement** — app-level keyword block + engine-level read-only transaction.
- **Dynamic database selection** — the agent picks the target database per query, not from config.
- **JSON output** — `{columns, rows}` response, easy to parse.
- **YAML config** via Viper with environment variable overrides.
- **Hexagonal architecture** — clean separation between domain, application, and platform layers.

## Requirements

- Go **1.26+** (build from source), or a prebuilt binary.
- A MySQL server reachable from the host running the MCP.

## Installation

### Option A: Build from source

```bash
go build -o gaz-mcp ./cmd/server/
```

### Option B: Prebuilt binary (no build required)

**Linux / macOS:**

```bash
# Latest release
curl -fsSL https://raw.githubusercontent.com/jcastilloa/gaz-mcp/master/scripts/install.sh | sh

# Specific version
curl -fsSL https://raw.githubusercontent.com/jcastilloa/gaz-mcp/master/scripts/install.sh | VERSION=vX.Y.Z sh
```

**Installer environment variables:**

| Variable | Default |
|---|---|
| `REPO` | `jcastilloa/gaz-mcp` |
| `SERVICE_NAME` | `gaz-mcp` |
| `INSTALL_DIR` | `~/.local/bin` |
| `VERSION` | Latest release tag |

### Option C: Agent skill (zero binary install)

When the MCP is already registered in the agent's MCP client config, drop `SKILL.md` into the agent's skills directory so it knows how to use the tool:

```bash
# Project-level (recommended for teams)
mkdir -p .claude/skills/gaz-mcp-db
cp SKILL.md .claude/skills/gaz-mcp-db/SKILL.md

# Global (available in all projects)
mkdir -p ~/.claude/skills/gaz-mcp-db
cp SKILL.md ~/.claude/skills/gaz-mcp-db/SKILL.md
```

| Agent | Project-level path | Global path |
|---|---|---|
| Claude Code | `.claude/skills/gaz-mcp-db/SKILL.md` | `~/.claude/skills/gaz-mcp-db/SKILL.md` |
| Codex | `.codex/skills/gaz-mcp-db/SKILL.md` | `~/.codex/skills/gaz-mcp-db/SKILL.md` |
| OpenCode | `.opencode/skills/gaz-mcp-db/SKILL.md` | `~/.opencode/skills/gaz-mcp-db/SKILL.md` |
| Cursor | `.cursor/skills/gaz-mcp-db/SKILL.md` | `~/.cursor/skills/gaz-mcp-db/SKILL.md` |

The skill teaches the agent the `sql_query` signature, usage patterns, read-only constraints, and best practices — so it knows exactly how to query your databases.

## Configuration

Create `config.yaml` in the working directory (or at `~/.config/gaz-mcp/config.yaml`):

```yaml
service:
  transport: stdio
  version: 0.1.0

mysql:
  host: 127.0.0.1
  port: 3306
  user: readonly_user
  password: your-password

postgres:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: your-password
```

All keys support environment variable overrides (`MYSQL_HOST`, `POSTGRES_HOST`, etc.).

The `database` is **not** configured statically — the AI agent passes it as a tool parameter per query.

## Quick Start

### 1. Configure

```bash
cp config.sample.yaml config.yaml
# Edit mysql.host, mysql.user, mysql.password
```

### 2. Run

```bash
go run ./cmd/server/ --transport stdio
# or with the built binary:
./gaz-mcp --transport stdio
```

### 3. Register in your MCP client

#### Claude Desktop / Cursor / JSON-based clients

```json
{
  "mcpServers": {
    "gaz-mcp": {
      "command": "/absolute/path/to/gaz-mcp",
      "args": ["--transport", "stdio"]
    }
  }
}
```

#### Codex (TOML)

```toml
[mcp_servers.gaz-mcp]
command = "/absolute/path/to/gaz-mcp"
args = ["--transport", "stdio"]
startup_timeout_sec = 20.0
```

#### Codex (CLI)

```bash
codex mcp add gaz-mcp -- /absolute/path/to/gaz-mcp --transport stdio
```

#### OpenCode

```bash
opencode mcp add
```

Follow the prompts: project or global → name `gaz-mcp` → type `local` → command `/absolute/path/to/gaz-mcp --transport stdio`.

> Always use an **absolute** binary path and `stdio` transport.

## MCP Tool Reference

### `sql_query`

Execute a read-only SQL query against MySQL or PostgreSQL.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `type` | string | no | Database engine: `mysql` or `postgres` (default: `mysql`) |
| `database` | string | yes | Database name to query |
| `query` | string | yes | SQL query — `SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN` |

**Returns:** JSON object with `columns` (string array) and `rows` (array of string arrays).

**Allowed statements:** `SELECT`, `SHOW` (MySQL), `DESCRIBE` (MySQL), `EXPLAIN`.

**Blocked statements:** `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, and anything that mutates data.

### Example agent interactions

```
sql_query(database="myapp", query="SHOW TABLES")
sql_query(database="myapp", query="DESCRIBE users")
sql_query(type="postgres", database="analytics", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public' LIMIT 20")
sql_query(type="postgres", database="analytics", query="SELECT * FROM users LIMIT 10")
```

## Architecture

```text
gaz-mcp/
├── cmd/server/              # Entry point
├── mcp/
│   ├── application/sql/     # Use case (read-only enforcement)
│   └── domain/sql/          # Domain port
├── platform/
│   ├── config/              # Viper config reader
│   ├── di/                  # Dependency injection
│   └── mcp/
│       ├── commands/        # Cobra CLI runner
│       ├── server/          # MCP server wrapper
│       ├── sql/             # MySQL + PostgreSQL adapters
│       └── tools/           # MCP tool definitions
└── shared/
    ├── ai/domain/           # AI provider contracts
    └── config/domain/       # Config contracts
```

**Dependency rule:** `platform → shared`, never reverse.

## Development

```bash
go build ./...     # Build
go vet ./...       # Static analysis
go test ./...      # Tests
```

## License

Copyright (c) 2026 jcastilloa.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
