---
name: gaz-mcp-db
description: Read-only MySQL and PostgreSQL MCP proxy. Use to explore schemas,
  describe tables, and run SELECT queries against development databases.
metadata:
  model: opus
---

You have access to the `gaz-mcp` SQL proxy — a read-only bridge to MySQL and PostgreSQL databases via MCP.

## Available tool

### `sql_query`

Executes a read-only query against any database the server can see.

**Parameters:**

| Parameter  | Required | Description |
|------------|----------|-------------|
| `type`     | no       | Database engine: `mysql` or `postgres` (default: `mysql`) |
| `database` | yes      | Database name to query |
| `query`    | yes      | SQL query — `SELECT`, `SHOW`, `DESCRIBE`, or `EXPLAIN` only |

**Returns:** JSON with `columns` (string array) and `rows` (array of string arrays).

## Usage patterns

MySQL — explore what tables exist:

```
sql_query(database="myapp", query="SHOW TABLES")
sql_query(database="myapp", query="DESCRIBE users")
sql_query(database="myapp", query="SELECT id, email FROM users LIMIT 20")
```

PostgreSQL — explore schemas:

```
sql_query(type="postgres", database="analytics", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public' LIMIT 20")
sql_query(type="postgres", database="analytics", query="SELECT column_name, data_type FROM information_schema.columns WHERE table_name='users' LIMIT 50")
```

Query with limits:

```
sql_query(type="postgres", database="analytics", query="SELECT id, email, created_at FROM users ORDER BY created_at DESC LIMIT 10")
```

## Read-only enforcement

Two layers of protection on every engine:

1. **Application layer** — rejects queries starting with `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, etc.
2. **Database layer** — MySQL: `SET SESSION TRANSACTION READ ONLY`. PostgreSQL: `SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY`.

You cannot write, modify, delete, or alter data through this MCP.

## Best practices

- Always add a `LIMIT` clause unless you intentionally need all rows.
- For MySQL: use `SHOW TABLES` and `DESCRIBE` to discover schema.
- For PostgreSQL: use `information_schema` or `pg_catalog` to explore schema.
- Prefer specific column names over `SELECT *`.
- The connection pool is small (3 max open connections). Keep queries focused.
