---
name: gaz-mcp-jenkins
description: Jenkins MCP proxy for gaz-mcp. Use to inspect jobs, builds, nodes,
  credentials, views, queue, plugins, and to trigger or modify Jenkins resources.
  Includes automatic config versioning (snapshots) before every write operation.
  For large responses (build logs, script output, config XML) always use
  distill_mcp_output from context-distill instead of reading the raw payload.
metadata:
  model: opus
---

You have access to the `gaz-mcp` Jenkins proxy — an MCP bridge to one or more Jenkins instances.

## Available environments

Check the tool description of any `jenkins_*` tool to see which environments are configured. Always use the exact environment name from that list.

---

## Tool reference

### Read tools (12)

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `jenkins_info` | `environment` | Version, job count, node count |
| `jenkins_job_list` | `environment`, `filter?` | List jobs with optional name filter |
| `jenkins_job_get` | `environment`, `name` | Job details |
| `jenkins_job_config` | `environment`, `name` | Raw config XML |
| `jenkins_build_info` | `environment`, `job_name`, `build_number` | Build result, duration, parameters |
| `jenkins_build_log` | `environment`, `job_name`, `build_number`, `start_line?` | Full console log (plain text) |
| `jenkins_build_artifacts` | `environment`, `job_name`, `build_number` | List artifacts |
| `jenkins_node_list` | `environment` | All nodes/agents |
| `jenkins_queue_list` | `environment` | Build queue |
| `jenkins_plugin_list` | `environment` | Installed plugins |
| `jenkins_view_list` | `environment` | All views |
| `jenkins_credential_list` | `environment`, `store?`, `domain?` | Credentials (IDs only — no secrets) |

### Write / execute tools (16)

> Every write tool that modifies a config XML automatically snapshots the current config before applying changes.

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `jenkins_job_set_config` | `environment`, `name`, `config_xml` | Update job config XML |
| `jenkins_job_create` | `environment`, `name`, `config_xml` | Create job |
| `jenkins_job_copy` | `environment`, `from`, `to` | Copy job |
| `jenkins_job_delete` | `environment`, `name` | Delete job (snapshots first) |
| `jenkins_job_enable` | `environment`, `name` | Enable job |
| `jenkins_job_disable` | `environment`, `name` | Disable job |
| `jenkins_job_build` | `environment`, `name`, `params?` | Trigger build |
| `jenkins_build_stop` | `environment`, `job_name`, `build_number` | Stop running build |
| `jenkins_queue_cancel` | `environment`, `id` | Cancel queued item |
| `jenkins_node_enable` | `environment`, `name` | Bring node online |
| `jenkins_node_disable` | `environment`, `name`, `message?` | Take node offline |
| `jenkins_script_console` | `environment`, `script` | Execute Groovy script |
| `jenkins_credential_create` | `environment`, `store`, `domain`, `id`, `config_xml` | Create credential |
| `jenkins_credential_delete` | `environment`, `store`, `domain`, `id` | Delete credential |
| `jenkins_view_create` | `environment`, `name`, `config_xml` | Create view |
| `jenkins_view_delete` | `environment`, `name` | Delete view |

### Snapshot / versioning tools (5)

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `jenkins_snapshot_list` | `environment`, `object_type`, `object_name`, `limit?` | List stored versions |
| `jenkins_snapshot_get` | `environment`, `object_type`, `object_name`, `version` | Get config XML for a version |
| `jenkins_snapshot_diff` | `environment`, `object_type`, `object_name`, `va`, `vb` | Get both XMLs for comparison |
| `jenkins_snapshot_restore` | `environment`, `object_type`, `object_name`, `version` | Restore to a previous version |
| `jenkins_snapshot_prune` | `environment`, `object_type`, `object_name`, `keep` | Delete old versions, keep newest N |

`object_type` values: `job`, `folder`, `view`, `node`, `credential`.

---

## Handling large responses with context-distill

Some Jenkins tools return large payloads — especially `jenkins_build_log`,
`jenkins_job_config`, and `jenkins_script_console`. **Never pass these raw to the LLM.**
Use `distill_mcp_output` from context-distill instead.

### The correct flow

```
agent → distill_mcp_output (context-distill) → jenkins_* (gaz-mcp) → compact answer
```

Context-distill calls gaz-mcp as an MCP client, gets the raw payload, and returns
only the distilled answer. The LLM never sees the raw log.

### When to distil

| Tool | Typical output size | Action |
|------|--------------------|--------|
| `jenkins_build_log` | Hundreds to thousands of lines | **Always use distill_mcp_output** |
| `jenkins_script_console` | Variable — can be very large | **When > 8 lines** |
| `jenkins_job_config` | XML — can be large | **When > 8 lines** |
| `jenkins_job_list` | Many jobs | **When > 8 lines** |
| `jenkins_build_info` | Short JSON | Only if verbose |
| `jenkins_info` | Short | Skip |

### Find errors in a build log

```
distill_mcp_output(
  server_command = "gaz-mcp",
  server_args    = ["--transport", "stdio"],
  tool_name      = "jenkins_build_log",
  tool_arguments = {
    "environment":  "production",
    "job":          "My Pipeline",
    "build_number": 42,
    "start_line":   0
  },
  question = "Return only error and exception lines as JSON array [{line_number, message}]."
)
```

### Check if a build passed or failed

```
distill_mcp_output(
  server_command = "gaz-mcp",
  server_args    = ["--transport", "stdio"],
  tool_name      = "jenkins_build_log",
  tool_arguments = {
    "environment":  "production",
    "job":          "My Pipeline",
    "build_number": 42
  },
  question = "Did the build pass? Return JSON: {result, total_tests, failing_tests, root_cause, affected_specs}"
)
```

### Summarise Groovy script output

```
distill_mcp_output(
  server_command = "gaz-mcp",
  server_args    = ["--transport", "stdio"],
  tool_name      = "jenkins_script_console",
  tool_arguments = {
    "environment": "production",
    "script":      "Jenkins.instance.getAllItems().collect { it.fullName }.join('\\n')"
  },
  question = "Return only job names that contain 'deploy', one per line."
)
```

### Output contract rules (mandatory)

1. **Every `distill_mcp_output` call MUST include an explicit output contract in `question`.**
   - Good: `"Return only error lines as JSON array [{line_number, message}]."`
   - Bad: `"What happened?"`
2. **One task per call.** Do not mix unrelated questions.
3. **Prefer machine-checkable formats**: PASS/FAIL, JSON, one-item-per-line.

---

## Usage patterns

### Investigate a failing build

```
# 1. Get build metadata (short — read directly)
jenkins_build_info(environment="production", job="deploy-api", build_number=99)

# 2. Get full log and distil for errors (large — always distil)
distill_mcp_output(
  server_command="gaz-mcp", server_args=["--transport","stdio"],
  tool_name="jenkins_build_log",
  tool_arguments={"environment":"production","job":"deploy-api","build_number":99},
  question="Return only error and exception lines as JSON [{line_number, message}]."
)
```

### Safe config update (with rollback capability)

```
# 1. Read current config
jenkins_job_config(environment="production", name="my-job")

# 2. Apply change (auto-snapshot taken before update)
jenkins_job_set_config(environment="production", name="my-job", config_xml="<project>...</project>")

# 3. If something goes wrong — list versions and restore
jenkins_snapshot_list(environment="production", object_type="job", object_name="my-job")
jenkins_snapshot_restore(environment="production", object_type="job", object_name="my-job", version=2)
```

### Explore all jobs

```
# If list is short — read directly
jenkins_job_list(environment="production")

# If list is long — distil
distill_mcp_output(
  server_command="gaz-mcp", server_args=["--transport","stdio"],
  tool_name="jenkins_job_list",
  tool_arguments={"environment":"production"},
  question="Return only job names that contain 'deploy', one per line."
)
```

---

## Security notes

- `jenkins_credential_list` returns credential IDs only — never secrets or passwords.
- `jenkins_script_console` executes arbitrary Groovy on the Jenkins master. Use with care.
- The `api_key` config field accepts either a **Jenkins API token** (recommended, generated in *User → Configure → API Token*) or a plain **password**. Always supply via environment variable — never hardcode.
