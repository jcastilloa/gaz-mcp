# AGENTS.md — gaz-mcp MCP

MySQL + PostgreSQL + Jenkins MCP proxy. Hexagonal architecture, `cobra` + `viper`, DI.

## Mandatory Skills

Before making changes in this repository, load and apply these local skills:

- `.codex/skills/gaz-mcp-db` — SQL proxy interface and usage patterns
- `.codex/skills/gaz-mcp-jenkins` — Jenkins proxy interface, usage patterns, and context-distill CLI integration
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
- `gaz-mcp-jenkins` documents the Jenkins proxy interface, all 33 tools, snapshot/versioning system, and how to use the `context-distill` CLI to distil large responses (build logs, script output, config XML).

## Structure

```text
gaz-mcp/
├── cmd/server/main.go
├── mcp/
│   ├── application/
│   │   ├── jenkins/          # Jenkins use cases + NoopSnapshotRepository
│   │   └── sql/
│   └── domain/
│       ├── jenkins/          # Repository + SnapshotRepository ports + domain types
│       └── sql/
├── platform/
│   ├── config/
│   ├── di/
│   ├── mcp/
│   │   ├── commands/         # Cobra runner + JenkinsTools struct
│   │   ├── jenkins/          # gojenkins infrastructure adapter
│   │   ├── server/
│   │   ├── snapshot/         # SQLite SnapshotRepository implementation
│   │   ├── sql/
│   │   └── tools/            # sql_query.go, jenkins_read.go, jenkins_write.go, jenkins_snapshot.go
│   └── openai/
└── shared/
    ├── ai/domain/
    └── config/domain/        # EnvironmentConfig, JenkinsEnvironmentConfig, SnapshotConfig
```

## MCP Tools

### SQL

| Tool        | Parameters                         | Description                                            |
|-------------|------------------------------------|--------------------------------------------------------|
| `sql_query` | `environment`, `database`, `query` | Read-only SQL across environments (MySQL + PostgreSQL) |

### Jenkins — Read (12 tools)

| Tool                      | Key parameters                              | Description                              |
|---------------------------|---------------------------------------------|------------------------------------------|
| `jenkins_info`            | `environment`                               | Version, job count, node count           |
| `jenkins_job_list`        | `environment`, `filter?`                    | List jobs with optional filter           |
| `jenkins_job_get`         | `environment`, `name`                       | Job details                              |
| `jenkins_job_config`      | `environment`, `name`                       | Raw config XML                           |
| `jenkins_build_info`      | `environment`, `job_name`, `build_number`   | Build result, duration, parameters       |
| `jenkins_build_log`       | `environment`, `job_name`, `build_number`   | Console log with optional start_line     |
| `jenkins_build_artifacts` | `environment`, `job_name`, `build_number`   | List artifacts                           |
| `jenkins_node_list`       | `environment`                               | All nodes/agents                         |
| `jenkins_queue_list`      | `environment`                               | Build queue                              |
| `jenkins_plugin_list`     | `environment`                               | Installed plugins                        |
| `jenkins_view_list`       | `environment`                               | All views                                |
| `jenkins_credential_list` | `environment`, `store?`, `domain?`          | Credentials (IDs only, no secrets)       |

### Jenkins — Write / Execute (16 tools)

> Write tools automatically snapshot the current config before applying changes.

| Tool                       | Key parameters                                        | Description                    |
|----------------------------|-------------------------------------------------------|--------------------------------|
| `jenkins_job_set_config`   | `environment`, `name`, `config_xml`                   | Update job config XML          |
| `jenkins_job_create`       | `environment`, `name`, `config_xml`                   | Create job                     |
| `jenkins_job_copy`         | `environment`, `from`, `to`                           | Copy job                       |
| `jenkins_job_delete`       | `environment`, `name`                                 | Delete job (snapshots first)   |
| `jenkins_job_enable`       | `environment`, `name`                                 | Enable job                     |
| `jenkins_job_disable`      | `environment`, `name`                                 | Disable job                    |
| `jenkins_job_build`        | `environment`, `name`, `params?`                      | Trigger build                  |
| `jenkins_build_stop`       | `environment`, `job_name`, `build_number`             | Stop running build             |
| `jenkins_queue_cancel`     | `environment`, `id`                                   | Cancel queued item             |
| `jenkins_node_enable`      | `environment`, `name`                                 | Bring node online              |
| `jenkins_node_disable`     | `environment`, `name`, `message?`                     | Take node offline              |
| `jenkins_script_console`   | `environment`, `script`                               | Execute Groovy script          |
| `jenkins_credential_create`| `environment`, `store`, `domain`, `id`, `config_xml`  | Create credential              |
| `jenkins_credential_delete`| `environment`, `store`, `domain`, `id`                | Delete credential              |
| `jenkins_view_create`      | `environment`, `name`, `config_xml`                   | Create view                    |
| `jenkins_view_delete`      | `environment`, `name`                                 | Delete view                    |

### Jenkins — Snapshots (5 tools)

| Tool                      | Key parameters                                              | Description                        |
|---------------------------|-------------------------------------------------------------|------------------------------------|
| `jenkins_snapshot_list`   | `environment`, `object_type`, `object_name`, `limit?`       | List stored versions               |
| `jenkins_snapshot_get`    | `environment`, `object_type`, `object_name`, `version`      | Get config XML for a version       |
| `jenkins_snapshot_diff`   | `environment`, `object_type`, `object_name`, `va`, `vb`     | Get both XMLs for comparison       |
| `jenkins_snapshot_restore`| `environment`, `object_type`, `object_name`, `version`      | Restore to a previous version      |
| `jenkins_snapshot_prune`  | `environment`, `object_type`, `object_name`, `keep`         | Delete old versions, keep newest N |

`object_type` values: `job`, `folder`, `view`, `node`, `credential`.

## MCP Convention

- Entry point: `cmd/server/main.go`.
- CLI flags managed with `cobra` and bound to `viper`.
- MCP server wiring in `platform/mcp/server/server.go`.
- Tool definitions in `platform/mcp/tools/`, registered in `platform/mcp/commands/root.go`.
- Jenkins tools are only registered when at least one Jenkins environment is configured.
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

Full config reference (see `config.sample.yaml`):

```yaml
service:
  transport: stdio
  version: 0.3.0

# SQL environments
environments:
  development:
    engine: mysql       # or "postgres"
    host: 127.0.0.1
    port: 3306
    user: root
    password: changeme

# Jenkins environments
jenkins:
  production:
    url: https://jenkins.example.com
    user: admin
    api_key: "${JENKINS_PROD_API_KEY}"   # always use env vars for secrets
    timeout: 30s
    insecure: false

# Automatic config versioning (SQLite, pure Go — no CGo)
snapshot:
  enabled: true
  db_path: ~/.config/gaz-mcp/jenkins_history.db
  max_versions: 50    # 0 = unlimited
  auto_prune: true
```

- The SQL `database` is selected dynamically per-query via the tool parameter — not from config.
- Jenkins `api_key` values are masked in all JSON output and logs (`MarshalJSON` override).
- Snapshot DB uses `modernc.org/sqlite` (pure Go, no CGo required).

## Snapshot / Versioning System

Every Jenkins write operation that modifies a config XML:

1. Reads the current config from Jenkins.
2. Computes SHA-256 checksum — skips snapshot if identical to latest.
3. Stores the **previous** config in SQLite with an auto-incremented version number.
4. Applies the new config to Jenkins.

`SnapshotRestore` takes a safety snapshot of the current state before restoring.

`NoopSnapshotRepository` (in `mcp/application/jenkins/`) is used when `snapshot.enabled: false`.

## Checklist

- [ ] New tools are registered in startup wiring (`platform/mcp/commands/root.go`).
- [ ] New Jenkins tools are gated behind `hasJenkins` check.
- [ ] Flags are bound via `viper` and support env overrides.
- [ ] No `platform` imports inside `shared` or `mcp/domain`.
- [ ] `go build ./...` and `go vet ./...` pass.
- [ ] `go test ./...` passes (unit + SQLite integration tests).
- [ ] Secrets (API keys, passwords) are never logged or returned in tool output.
