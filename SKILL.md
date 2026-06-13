---
name: gaz-mcp-db
description: Read-only MySQL MCP proxy. Use to explore schemas, describe tables,
  and run SELECT queries against development databases.
metadata:
  model: opus
---

You have access to the `gaz-mcp` SQL proxy — a read-only MySQL bridge available via MCP.

## Available tool

### `sql_query`

Executes a read-only query against any database the MySQL server can see.

**Parameters:**

| Parameter  | Required | Description |
|------------|----------|-------------|
| `database` | yes      | MySQL database name to query |
| `query`    | yes      | SQL query — `SELECT`, `SHOW`, `DESCRIBE`, or `EXPLAIN` only |

**Returns:** JSON with `columns` (string array) and `rows` (array of string arrays).

## Usage patterns

Explore what tables exist:

```
sql_query(database="myapp", query="SHOW TABLES")
```

Inspect a table schema:

```
sql_query(database="myapp", query="DESCRIBE users")
sql_query(database="myapp", query="SHOW CREATE TABLE users")
```

Query with limits:

```
sql_query(database="myapp", query="SELECT id, email, status FROM users LIMIT 20")
```

Join across tables:

```
sql_query(database="myapp", query="SELECT u.name, COUNT(o.id) AS orders FROM users u LEFT JOIN orders o ON o.user_id = u.id GROUP BY u.id LIMIT 50")
```

## Read-only enforcement

Two layers of protection:

1. **Application layer** — rejects queries starting with `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, etc.
2. **MySQL layer** — every query runs inside `SET SESSION TRANSACTION READ ONLY`, so the database itself rejects any write attempt.

You cannot write, modify, delete, or alter data through this MCP.

## Best practices

- Always add a `LIMIT` clause unless you intentionally need all rows.
- Use `SHOW TABLES` and `DESCRIBE` to discover schema before writing complex queries.
- Prefer specific column names over `SELECT *`.
- The connection pool is small (3 max open connections). Keep queries focused.
