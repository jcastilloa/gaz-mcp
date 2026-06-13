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

Executes a read-only query. The tool description lists all available environments.

**Parameters:**

| Parameter     | Required | Description |
|---------------|----------|-------------|
| `environment` | yes      | Environment name as shown in the tool description |
| `database`    | yes      | Database name to query |
| `query`       | yes      | SQL query — `SELECT`, `SHOW`, `DESCRIBE`, or `EXPLAIN` |

**Returns:** JSON with `columns` (string array) and `rows` (array of string arrays).

## Usage patterns

MySQL — explore what tables exist:

```
sql_query(environment="dev1", database="myapp", query="SHOW TABLES")
sql_query(environment="dev1", database="myapp", query="DESCRIBE users")
sql_query(environment="dev1", database="myapp", query="SELECT id, email FROM users LIMIT 20")
```

PostgreSQL — explore schema via system catalogs:

```
sql_query(environment="analytics", database="analytics", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public' LIMIT 20")
sql_query(environment="analytics", database="analytics", query="SELECT column_name, data_type FROM information_schema.columns WHERE table_name='users'")
```

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
