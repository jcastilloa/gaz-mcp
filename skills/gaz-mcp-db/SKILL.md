---
name: gaz-mcp-db
description: Use the gaz-mcp read-only SQL proxy to inspect MySQL or PostgreSQL databases, discover schemas, explain queries, and retrieve focused data safely. Use when a configured gaz-mcp server exposes sql_query and the task requires database exploration or analysis without writes.
---

# gaz-mcp SQL

Use `sql_query` to inspect a configured MySQL or PostgreSQL environment. The tool is intentionally read-only.

## Tool contract

Call `sql_query` with:

| Parameter | Meaning |
|---|---|
| `environment` | Exact environment name advertised by the tool description |
| `database` | Database to inspect |
| `query` | One read-only `SELECT`, `SHOW`, `DESCRIBE`, or `EXPLAIN` statement |

The result contains `columns` and `rows`, with row values encoded as strings.

## Workflow

1. Read the tool description and select an advertised environment. Never guess its name.
2. Discover the schema before assuming table or column names.
3. Run the smallest query that answers the question.
4. Interpret the result with the engine's semantics in mind.

For MySQL, start with:

```text
sql_query(environment="development", database="app", query="SHOW TABLES")
sql_query(environment="development", database="app", query="DESCRIBE users")
```

For PostgreSQL, use catalogs:

```text
sql_query(environment="analytics", database="reporting", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' LIMIT 50")
sql_query(environment="analytics", database="reporting", query="SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'users' ORDER BY ordinal_position")
```

Then query explicit columns with a limit:

```text
sql_query(environment="development", database="app", query="SELECT id, email FROM users ORDER BY id DESC LIMIT 20")
```

## Operating rules

- Add `LIMIT` unless the complete result is necessary.
- Prefer explicit columns over `SELECT *`.
- Use `EXPLAIN` before making performance claims; do not use mutating variants such as `EXPLAIN ANALYZE` when they could execute writes.
- Keep exploratory queries focused because environments use a small connection pool.
- Do not attempt DDL, DML, transactions, session changes, stored procedures, or administrative commands. The proxy and database session enforce read-only access.
- Never claim a table, relationship, or performance cause that the returned schema or plan does not support.
