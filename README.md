# gaz-mcp

MySQL, PostgreSQL and Jenkins proxy exposed as an MCP server. Lets AI coding agents explore schemas, run `SELECT` queries safely, and interact with Jenkins CI/CD pipelines — with automatic config versioning before every write.

## Overview

`gaz-mcp` wraps MySQL, PostgreSQL and Jenkins connections in an MCP-compatible stdio server. AI agents call the `sql_query` tool to inspect tables and query data, and the `jenkins_*` family of tools to manage jobs, builds, nodes, views, credentials and more.

Each SQL environment bundles engine type + connection credentials. Each Jenkins environment bundles URL + credentials. The agent only needs to know the environment name — the engine is transparent.

## Features

- **MCP stdio server** — one binary, no daemon, no network ports.
- **Multi-environment** — configure MySQL, PostgreSQL and Jenkins environments; switch per query/call.
- **Read-only SQL enforcement** — app-level keyword block + engine-level read-only transaction.
- **Dynamic database selection** — the agent picks the target database per query, not from config.
- **Jenkins proxy** — 33 tools covering jobs, builds, nodes, views, queue, plugins, credentials and script console.
- **Automatic config versioning** — every Jenkins write operation snapshots the previous config to SQLite before applying changes. Restore any object to any previous version.
- **SHA-256 deduplication** — identical consecutive snapshots are skipped automatically.
- **JSON output** — all tools return structured JSON, easy to parse.
- **YAML config** via Viper with environment variable overrides (API keys never appear in logs).
- **Hexagonal architecture** — clean separation between domain, application, and platform layers.

## Requirements

- Go **1.22+** (build from source), or a prebuilt binary.
- A MySQL/PostgreSQL server and/or Jenkins instance reachable from the host running the MCP.

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
  version: 0.3.0

# SQL environments (MySQL + PostgreSQL)
environments:
  dev1:
    engine: mysql
    host: 127.0.0.1
    port: 3306
    user: readonly_user
    password: your-password

  analytics:
    engine: postgres
    host: 127.0.0.1
    port: 5432
    user: postgres
    password: your-password

# Jenkins environments
jenkins:
  production:
    url: https://jenkins.example.com
    user: admin
    api_key: "${JENKINS_PROD_API_KEY}"   # Jenkins API token or password — always use env vars
    timeout: 30s
    insecure: false
  staging:
    url: https://jenkins-staging.example.com
    user: admin
    api_key: "${JENKINS_STAGING_API_KEY}"
    timeout: 30s
    insecure: true                        # allow self-signed TLS

# Automatic config versioning (SQLite)
snapshot:
  enabled: true
  db_path: ~/.config/gaz-mcp/jenkins_history.db
  max_versions: 50    # keep last 50 versions per object (0 = unlimited)
  auto_prune: true
```

All keys support environment variable overrides. `engine` defaults to `mysql` when omitted. The `database` is passed per SQL query, not configured statically.

> **Security:** The `api_key` field accepts either a **Jenkins API token** (recommended — generated in *User → Configure → API Token*) or a plain **password**. Always supply secrets via environment variables (e.g. `JENKINS_PROD_API_KEY`). Values are masked in all JSON output and logs.

## Quick Start

### 1. Configure

```bash
cp config.sample.yaml config.yaml
# Edit environments, jenkins, and snapshot sections
export JENKINS_PROD_API_KEY=your-api-key
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

Execute a read-only SQL query against a configured environment.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `environment` | string | yes | Environment name (e.g. `dev1`, `analytics`) |
| `database` | string | yes | Database name to query |
| `query` | string | yes | SQL query — `SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN` |

**Returns:** JSON object with `columns` (string array) and `rows` (array of string arrays).

**Allowed statements:** `SELECT`, `SHOW` (MySQL), `DESCRIBE` (MySQL), `EXPLAIN`.

**Blocked statements:** `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, and anything that mutates data.

### Jenkins tools

All Jenkins tools require an `environment` parameter matching a key in the `jenkins:` config section.

#### Read tools

| Tool | Key parameters | Description |
|---|---|---|
| `jenkins_info` | `environment` | Jenkins version, job count, node count, quiet mode |
| `jenkins_job_list` | `environment`, `filter?` | List jobs, optional substring filter |
| `jenkins_job_get` | `environment`, `name` | Job details (status, last build, health) |
| `jenkins_job_config` | `environment`, `name` | Raw config XML for a job |
| `jenkins_build_info` | `environment`, `job_name`, `build_number` | Build details (result, duration, parameters) |
| `jenkins_build_log` | `environment`, `job_name`, `build_number`, `start_line?` | Console log with optional offset |
| `jenkins_build_artifacts` | `environment`, `job_name`, `build_number` | List build artifacts |
| `jenkins_node_list` | `environment` | All nodes/agents with online/idle status |
| `jenkins_queue_list` | `environment` | Current build queue |
| `jenkins_plugin_list` | `environment` | Installed plugins with versions |
| `jenkins_view_list` | `environment` | All views with job lists |
| `jenkins_credential_list` | `environment`, `store?`, `domain?` | Credentials (IDs only, no secrets) |

#### Write / execute tools

> ⚠️ Write tools automatically snapshot the current config before applying changes.

| Tool | Key parameters | Description |
|---|---|---|
| `jenkins_job_set_config` | `environment`, `name`, `config_xml` | Update job config XML |
| `jenkins_job_create` | `environment`, `name`, `config_xml` | Create a new job |
| `jenkins_job_copy` | `environment`, `from`, `to` | Copy an existing job |
| `jenkins_job_delete` | `environment`, `name` | Delete a job (snapshots first) |
| `jenkins_job_enable` | `environment`, `name` | Enable a disabled job |
| `jenkins_job_disable` | `environment`, `name` | Disable a job |
| `jenkins_job_build` | `environment`, `name`, `params?` | Trigger a build with optional parameters |
| `jenkins_build_stop` | `environment`, `job_name`, `build_number` | Stop a running build |
| `jenkins_queue_cancel` | `environment`, `id` | Cancel a queued item |
| `jenkins_node_enable` | `environment`, `name` | Bring a node back online |
| `jenkins_node_disable` | `environment`, `name`, `message?` | Take a node offline |
| `jenkins_script_console` | `environment`, `script` | Execute Groovy script (Jenkins Script Console) |
| `jenkins_credential_create` | `environment`, `store`, `domain`, `id`, `config_xml` | Create a credential |
| `jenkins_credential_delete` | `environment`, `store`, `domain`, `id` | Delete a credential |
| `jenkins_view_create` | `environment`, `name`, `config_xml` | Create a view |
| `jenkins_view_delete` | `environment`, `name` | Delete a view |

#### Snapshot / versioning tools

| Tool | Key parameters | Description |
|---|---|---|
| `jenkins_snapshot_list` | `environment`, `object_type`, `object_name`, `limit?`, `offset?` | List stored versions |
| `jenkins_snapshot_get` | `environment`, `object_type`, `object_name`, `version` | Get config XML for a specific version |
| `jenkins_snapshot_diff` | `environment`, `object_type`, `object_name`, `version_a`, `version_b` | Get both XMLs for comparison |
| `jenkins_snapshot_restore` | `environment`, `object_type`, `object_name`, `version` | Restore object to a previous version |
| `jenkins_snapshot_prune` | `environment`, `object_type`, `object_name`, `keep` | Delete old versions, keep newest N |

`object_type` values: `job`, `folder`, `view`, `node`, `credential`.

### Example agent interactions

```
# SQL
sql_query(environment="dev1", database="myapp", query="SHOW TABLES")
sql_query(environment="analytics", database="reporting", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public'")

# Jenkins — read
jenkins_info(environment="production")
jenkins_job_list(environment="staging", filter="deploy")
jenkins_build_log(environment="production", job_name="backend-ci", build_number=42)

# Jenkins — write (auto-snapshots before change)
jenkins_job_set_config(environment="staging", name="my-job", config_xml="<project>...</project>")
jenkins_job_build(environment="production", name="deploy-app", params={"BRANCH": "main"})

# Jenkins — restore a previous config
jenkins_snapshot_list(environment="production", object_type="job", object_name="my-job")
jenkins_snapshot_restore(environment="production", object_type="job", object_name="my-job", version=3)
```

## Architecture

```text
gaz-mcp/
├── cmd/server/              # Entry point
├── mcp/
│   ├── application/
│   │   ├── jenkins/         # Jenkins use cases + NoopSnapshotRepository
│   │   └── sql/             # SQL use case (read-only enforcement)
│   └── domain/
│       ├── jenkins/         # Domain ports: Repository + SnapshotRepository
│       └── sql/             # Domain port: Repository
├── platform/
│   ├── config/              # Viper config reader
│   ├── di/                  # Dependency injection container
│   └── mcp/
│       ├── commands/        # Cobra CLI runner + JenkinsTools wiring
│       ├── jenkins/         # gojenkins infrastructure adapter
│       ├── server/          # MCP server wrapper
│       ├── snapshot/        # SQLite snapshot repository
│       ├── sql/             # MySQL + PostgreSQL adapters
│       └── tools/           # MCP tool definitions (sql, jenkins_read/write/snapshot)
└── shared/
    ├── ai/domain/           # AI provider contracts
    └── config/domain/       # Config contracts (EnvironmentConfig, JenkinsEnvironmentConfig, SnapshotConfig)
```

**Dependency rule:** `platform → shared + mcp/application + mcp/domain`. Never reverse.

## Development

```bash
go build ./...                                    # Build all packages
go vet ./...                                      # Static analysis
go test ./...                                     # All tests
go test ./mcp/application/jenkins/... -v          # Jenkins service unit tests
go test ./platform/mcp/snapshot/... -v            # SQLite snapshot integration tests
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
