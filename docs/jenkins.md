# Jenkins

`gaz-mcp` exposes Jenkins inspection, administration, and configuration-history tools for each configured Jenkins environment. There are 33 Jenkins tools: 12 read tools, 16 write or execution tools, and 5 snapshot tools.

For installation, client registration, and shared configuration, see the [root README](../README.md).

## Configuration

Each key under `jenkins` names an environment that callers select through the required `environment` parameter.

```yaml
jenkins:
  production:
    url: https://jenkins.example.com
    user: admin
    api_key: "${JENKINS_PROD_API_KEY}"
    timeout: 30s
    insecure: false

  staging:
    url: https://jenkins-staging.example.com
    user: admin
    api_key: "${JENKINS_STAGING_API_KEY}"
    timeout: 30s
    insecure: true # allow self-signed TLS

snapshot:
  enabled: true
  db_path: ~/.config/gaz-mcp/jenkins_history.db
  max_versions: 50 # positive retention limit per object
  auto_prune: true
```

The Jenkins tools are registered only when at least one Jenkins environment is configured. Check a `jenkins_*` tool description for the exact environment names available at runtime.

The `${...}` value is a deployment template. The current configuration reader does not expand it itself, so resolve the secret through deployment tooling before `gaz-mcp` starts. `api_key` accepts a Jenkins API token (recommended) or a password; it is masked in JSON output and logs. `insecure: true` accepts a self-signed TLS certificate and should be restricted to environments where that is intended.

## Tool reference

Every tool below requires `environment`. Job-oriented tools use `job`, including the full Jenkins path for nested jobs. Required parameters are shown without `?`; optional parameters end in `?`.

### Read tools

| Tool | Parameters | Description |
|---|---|---|
| `jenkins_info` | `environment` | Jenkins version, job count, node count, and quiet-mode state. |
| `jenkins_job_list` | `environment`, `filter?` | List jobs; `filter` is a substring filter. |
| `jenkins_job_get` | `environment`, `job` | Job details, including status, last build, and health. |
| `jenkins_job_config` | `environment`, `job` | Raw job configuration XML. |
| `jenkins_build_info` | `environment`, `job`, `build_number` | Build result, duration, parameters, and metadata. |
| `jenkins_build_log` | `environment`, `job`, `build_number`, `start_line?` | Console log, optionally starting at a line offset. |
| `jenkins_build_artifacts` | `environment`, `job`, `build_number` | Artifacts of a build. |
| `jenkins_node_list` | `environment` | Nodes/agents and their status. |
| `jenkins_queue_list` | `environment` | Current build queue. |
| `jenkins_plugin_list` | `environment` | Installed plugins and versions. |
| `jenkins_view_list` | `environment` | Views and their job lists. |
| `jenkins_credential_list` | `environment`, `store?`, `domain?` | Credential identifiers and metadata only; never secret values. |

### Write and execution tools

| Tool | Parameters | Description |
|---|---|---|
| `jenkins_job_set_config` | `environment`, `job`, `config_xml` | Replace a job configuration XML. |
| `jenkins_job_create` | `environment`, `job`, `config_xml` | Create a job from configuration XML. |
| `jenkins_job_copy` | `environment`, `from`, `to` | Copy an existing job. |
| `jenkins_job_delete` | `environment`, `job` | Delete a job. |
| `jenkins_job_enable` | `environment`, `job` | Enable a disabled job. |
| `jenkins_job_disable` | `environment`, `job` | Disable a job. |
| `jenkins_job_build` | `environment`, `job`, `params?` | Trigger a build. `params` is a JSON-encoded string mapping parameter names to values. |
| `jenkins_build_stop` | `environment`, `job`, `build_number` | Stop a running build. |
| `jenkins_queue_cancel` | `environment`, `queue_id` | Cancel a queued item. |
| `jenkins_node_enable` | `environment`, `node` | Bring a node online. |
| `jenkins_node_disable` | `environment`, `node`, `message?` | Take a node offline, with an optional message. |
| `jenkins_script_console` | `environment`, `script` | Execute Groovy in the Jenkins Script Console. |
| `jenkins_credential_create` | `environment`, `id`, `config_xml`, `store?`, `domain?` | Create a credential from configuration XML. |
| `jenkins_credential_delete` | `environment`, `id`, `store?`, `domain?` | Delete a credential. |
| `jenkins_view_create` | `environment`, `name`, `config_xml` | Create a view from configuration XML. |
| `jenkins_view_delete` | `environment`, `name` | Delete a view. |

`jenkins_script_console` can execute arbitrary Groovy with Jenkins controller privileges. Treat it as a privileged remote-execution operation and use it only when that authority is deliberate.

### Snapshot and versioning tools

| Tool | Parameters | Description |
|---|---|---|
| `jenkins_snapshot_list` | `environment`, `object_type`, `object_name`, `limit?`, `offset?` | List stored configuration versions. |
| `jenkins_snapshot_get` | `environment`, `object_type`, `object_name`, `version` | Get XML stored for a version. |
| `jenkins_snapshot_diff` | `environment`, `object_type`, `object_name`, `version_a`, `version_b` | Retrieve two stored XMLs for a comparison. |
| `jenkins_snapshot_restore` | `environment`, `object_type`, `object_name`, `version` | Restore a stored configuration version. |
| `jenkins_snapshot_prune` | `environment`, `object_type`, `object_name`, `keep` | Delete older versions while keeping the newest `keep`. |

Valid `object_type` values are `job`, `folder`, `view`, `node`, and `credential`.

## Configuration history

Snapshots are stored in SQLite through the pure-Go `modernc.org/sqlite` driver; CGo is not required. The repository assigns increasing version numbers and uses a SHA-256 checksum to avoid storing an identical consecutive configuration.

The current write coverage is intentional and explicit:

- Job configuration updates store the previous configuration before applying a change. Job creation and copying store the created configuration, and job deletion stores the configuration before deletion.
- View and credential creation store the created configuration.
- View and credential deletion do not currently create a snapshot, because no pre-delete configuration capture is implemented for those operations.
- A restore takes a safety snapshot before it writes and currently supports `job` and `folder`; snapshots for other types can still be listed, read, diffed, and pruned, but cannot be restored by this implementation.

When `snapshot.enabled` is false, write operations continue without durable history and snapshot retrieval/restore cannot provide a version. `max_versions` is the effective retention limit and the current implementation enforces it after storing a snapshot. Although the domain permits zero to mean unlimited, the current configuration reader normalises zero and other non-positive YAML values to its default of 50; use a positive value in configuration. The sample `auto_prune` key remains accepted as configuration, but retention is governed by `max_versions`.

## Workflows

### Investigate a failing build

```text
jenkins_build_info(environment="production", job="deploy-api", build_number=99)
jenkins_build_log(environment="production", job="deploy-api", build_number=99, start_line=0)
jenkins_build_artifacts(environment="production", job="deploy-api", build_number=99)
```

Read the short build metadata first. Build logs are usually large, so use the large-output process below before asking an agent to inspect one.

### Update a job with a rollback path

```text
jenkins_job_config(environment="production", job="my-job")
jenkins_job_set_config(environment="production", job="my-job", config_xml="<project>...</project>")
jenkins_snapshot_list(environment="production", object_type="job", object_name="my-job")
jenkins_snapshot_restore(environment="production", object_type="job", object_name="my-job", version=2)
```

The version list is the authoritative source for the version to restore; do not assume a version number.

### Trigger a parameterised build

```text
jenkins_job_build(environment="production", job="deploy-app", params="{\"BRANCH\":\"main\"}")
```

`params` is a string containing valid JSON, not an MCP object parameter.

### Explore jobs selectively

```text
jenkins_job_list(environment="staging", filter="deploy")
jenkins_job_get(environment="staging", job="folder/deploy-api")
```

## Handling large responses

`jenkins_build_log`, `jenkins_job_config`, and `jenkins_script_console` can produce large payloads. A long job list can too. Do not give raw large outputs to an LLM; ask `context-distill` to call the MCP tool and return a constrained answer instead.

Confirm the helper once per session before using it:

```bash
which context-distill
context-distill distill_mcp_output --help
```

If it is not installed, install it from the `context-distill` repository with `make install` or with:

```bash
go install github.com/jcastilloa/context-distill/cmd/server@latest
```

The flow is:

```text
agent → distill_mcp_output (context-distill) → jenkins_* (gaz-mcp) → compact answer
```

For example, request only the errors from a build log:

```text
distill_mcp_output(
  server_command="gaz-mcp",
  server_args=["--transport", "stdio"],
  tool_name="jenkins_build_log",
  tool_arguments={"environment":"production","job":"deploy-api","build_number":99},
  question="Return only error and exception lines as JSON array [{line_number, message}]."
)
```

Each distillation question must state an output contract. Keep one task per call and prefer JSON, `PASS`/`FAIL`, or one item per line. Directly read short JSON such as `jenkins_info` or concise `jenkins_build_info`; distil a script result, configuration XML, job list, or log when it exceeds a few lines.

## Operational and security notes

- Use the exact environment name exposed by the runtime tool description.
- `jenkins_credential_list` deliberately returns IDs and metadata, never passwords or secrets.
- Review XML before writing it. A valid XML document can still make a Jenkins job unsafe or inoperative.
- Take `snapshot.enabled` and actual restore coverage into account before relying on a rollback path.
- Jenkins permissions still apply to the configured account; grant the minimum permissions needed for the tools that account must use.

For SQL inspection and Go process profiling, return to the [documentation map](../README.md#documentation-map).
