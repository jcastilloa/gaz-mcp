---
name: gaz-mcp-jenkins
description: Use the gaz-mcp Jenkins proxy to inspect jobs, builds, logs, artifacts, nodes, queue items, plugins, views, credentials, and configuration snapshots, or to perform explicitly requested Jenkins changes. Use when a configured gaz-mcp server exposes jenkins_* tools and the task concerns Jenkins operations, troubleshooting, or rollback.
---

# gaz-mcp Jenkins

Operate Jenkins through the available `jenkins_*` tools. Read each tool's runtime schema for exact parameters and configured environments.

## Start safely

1. Select an exact environment advertised by the tool description; never invent one.
2. Prefer read tools while investigating.
3. Before a write, inspect the current object and verify that the requested environment and object are unambiguous.
4. Perform only the mutation the user requested, then verify the resulting state.

Tool families are:

- Server and inventory: `jenkins_info`, job, node, queue, plugin, view, and credential list/get tools.
- Builds: build information, logs, artifacts, triggering, stopping, and queue cancellation.
- Configuration: create, copy, update, enable, disable, and delete jobs; create/delete views and credentials; enable/disable nodes.
- Snapshots: list, get, diff, restore, and prune stored configuration versions.
- Privileged execution: `jenkins_script_console` runs arbitrary Groovy on the controller.

## Handle large output

Do not load a full build log, large XML configuration, long job list, or large script result directly when `context-distill` is available. Confirm its interface once per session:

```bash
command -v context-distill
context-distill distill_mcp_output --help
```

Then have it invoke gaz-mcp and return only the required evidence:

```text
distill_mcp_output(
  server_command="gaz-mcp",
  server_args=["--transport", "stdio"],
  tool_name="jenkins_build_log",
  tool_arguments={"environment":"production","job":"deploy-api","build_number":99},
  question="Return error and exception lines only as JSON [{line_number, message}]."
)
```

Every distillation request must contain one question and an explicit output contract. Read short responses such as server or build metadata directly. If `context-distill` is unavailable, use focused tool parameters such as `filter` or `start_line` and avoid reproducing large raw payloads.

## Common workflows

Investigate a failed build:

1. Read build metadata.
2. Distil the console log for the failure and nearby context.
3. Inspect relevant artifacts or job configuration only if the evidence points there.
4. Report observed evidence separately from inferred root cause.

Change a job configuration:

1. Read the current job and configuration.
2. Apply the smallest XML change with the appropriate set-config tool.
3. Read the job/configuration again to verify it.
4. If verification fails, list snapshots and restore only with explicit authority.

Use snapshots:

- List versions before choosing one.
- Get or diff versions before restoring.
- Treat restore and prune as writes; prune permanently deletes history.
- Do not assume every operation is reversible. Snapshot capture and restore support vary by object type and operation.

## Security rules

- Credential tools expose identifiers and metadata, not secret values. Never request or reveal secrets.
- Treat the script console as privileged remote code execution and use it only when explicitly required.
- Review configuration XML before submitting it; syntactically valid XML can still make a job unsafe or unusable.
- Triggering or stopping builds, cancelling queue items, changing nodes, restoring snapshots, and all create/update/delete operations change external state. Do not perform them for a read-only request.
