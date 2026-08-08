# Agent skills shipped with gaz-mcp

This directory contains the skills intended for agents that use a running `gaz-mcp` server. They are product assets, separate from the repository-development skills in `.codex/skills`.

| Skill | Use it for |
|---|---|
| [`gaz-mcp-db`](gaz-mcp-db/SKILL.md) | Read-only MySQL and PostgreSQL inspection through `sql_query` |
| [`gaz-mcp-jenkins`](gaz-mcp-jenkins/SKILL.md) | Jenkins inspection, administration, snapshots, and large-output handling |
| [`gaz-mcp-diagnostics`](gaz-mcp-diagnostics/SKILL.md) | Go service instrumentation and process diagnosis with pprof, expvar, metrics, and heuristic suspicions |

Each skill is concise and self-contained: `SKILL.md` contains the operating knowledge required by the agent and does not depend on this README or the product manuals. `agents/openai.yaml` supplies optional UI metadata for clients that support it.

## Installation

Copy the complete directory for each capability the agent may use. The target must be the project that consumes gaz-mcp, not this repository's `.codex/skills` development directory. For example:

```bash
GAZ_MCP_DIR=/path/to/gaz-mcp
CONSUMER_PROJECT=/path/to/project-using-gaz-mcp
mkdir -p "$CONSUMER_PROJECT/.codex/skills"
cp -R "$GAZ_MCP_DIR/skills/gaz-mcp-db" "$CONSUMER_PROJECT/.codex/skills/"
cp -R "$GAZ_MCP_DIR/skills/gaz-mcp-jenkins" "$CONSUMER_PROJECT/.codex/skills/"
cp -R "$GAZ_MCP_DIR/skills/gaz-mcp-diagnostics" "$CONSUMER_PROJECT/.codex/skills/"
```

Use the equivalent skill root for another client:

| Client | Project-level root | User-level root |
|---|---|---|
| Claude Code | `.claude/skills` | `~/.claude/skills` |
| Codex | `.codex/skills` | `~/.codex/skills` |
| OpenCode | `.opencode/skills` | `~/.opencode/skills` |
| Cursor | `.cursor/skills` | `~/.cursor/skills` |

Install only the skills matching the enabled gaz-mcp capabilities. Copy the whole skill directory rather than only `SKILL.md`, so present and future metadata remains intact.

## Repository boundary

- `skills/` is the distributable product catalog for users of gaz-mcp.
- `.codex/skills/` contains local instructions for contributors developing gaz-mcp.
- `.claude/skills` and `.opencode/skills` are repository-local symlinks to `.codex/skills`; they expose development skills, not the product catalog.

Do not add product-facing skills to `.codex/skills` and do not add contributor workflow rules to `skills/`.
