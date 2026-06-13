---
name: gaz-mcp-db
description: Read-only SQL MCP proxy for database environments. Use to explore schemas,
  describe tables, and run SELECT queries against MySQL and PostgreSQL.
metadata:
  model: opus
---

You have access to the `gaz-mcp` SQL proxy — a read-only MCP bridge to MySQL and PostgreSQL databases.

## Available tool

### `sql_query`

Executes a read-only query. The tool description lists all available environments configured on the server.

**Parameters:**

| Parameter     | Required | Description |
|---------------|----------|-------------|
| `environment` | yes      | Environment name — check the tool description for available values |
| `database`    | yes      | Database name to query |
| `query`       | yes      | SQL query — `SELECT`, `SHOW`, `DESCRIBE`, or `EXPLAIN` |

**Returns:** JSON with `columns` (string array) and `rows` (array of string arrays).

## Usage patterns

Always check the `sql_query` tool description first to see which environments are available. Then use the exact environment name from that list.

MySQL — explore what tables exist:

```
sql_query(environment="<name-from-tool-description>", database="myapp", query="SHOW TABLES")
sql_query(environment="<name-from-tool-description>", database="myapp", query="DESCRIBE users")
sql_query(environment="<name-from-tool-description>", database="myapp", query="SELECT id, email FROM users LIMIT 20")
```

PostgreSQL — explore schema via system catalogs:

```
sql_query(environment="<name-from-tool-description>", database="analytics", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public' LIMIT 20")
sql_query(environment="<name-from-tool-description>", database="analytics", query="SELECT column_name, data_type FROM information_schema.columns WHERE table_name='users'")
```

The engine (MySQL or PostgreSQL) is determined by the environment configuration — you don't need to specify it.

## Read-only enforcement

Two layers of protection:

1. **Application layer** — rejects queries starting with `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, etc.
2. **Database layer** — MySQL: `SET SESSION TRANSACTION READ ONLY`. PostgreSQL: `SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY`.

You cannot write, modify, delete, or alter data through this MCP.

## Best practices

- Always add a `LIMIT` clause unless you intentionally need all rows.
- For MySQL: use `SHOW TABLES` and `DESCRIBE` to discover schema.
- For PostgreSQL: use `information_schema` or `pg_catalog` to explore schema.
- Prefer specific column names over `SELECT *`.
- The connection pool is small (3 max open connections per environment). Keep queries focused.
