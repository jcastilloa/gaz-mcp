# SQL: MySQL and PostgreSQL

`gaz-mcp` exposes one read-only tool, `sql_query`, for every SQL environment configured in `environments`. The caller selects both the environment and database for each request; the configured engine is resolved automatically.

For installation and shared configuration, see the [root README](../README.md).
Agents using this capability can install the self-contained [`gaz-mcp-db` skill](../skills/gaz-mcp-db/SKILL.md).

## Configuration

Configure one or more environments. `engine` is `mysql` by default; use `postgres` for PostgreSQL.

```yaml
environments:
  development:
    engine: mysql
    host: 127.0.0.1
    port: 3306
    user: readonly_user
    password: "${MYSQL_DEVELOPMENT_PASSWORD}"

  analytics:
    engine: postgres
    host: 127.0.0.1
    port: 5432
    user: readonly_user
    password: "${POSTGRES_ANALYTICS_PASSWORD}"
```

The database is not part of this configuration. Supplying it per request lets the same connection environment inspect multiple databases without duplicating credentials.

The `${...}` strings are deployment templates. The current configuration reader does not expand them itself, so resolve them through your deployment or secret-management process before launching the server.

## Tool reference

### `sql_query`

Execute a read-only query against a configured environment.

| Parameter | Type | Required | Description |
|---|---|---:|---|
| `environment` | string | yes | A configured environment name. Check the tool description for the values available at runtime. |
| `database` | string | yes | Database to inspect. |
| `query` | string | yes | Read-only SQL statement. |

The result is JSON with `columns` (an array of strings) and `rows` (an array of string arrays).

Use `SELECT`, `SHOW`, `DESCRIBE`, and `EXPLAIN` as appropriate for the configured engine. `SHOW` and `DESCRIBE` are MySQL discovery statements; PostgreSQL catalog queries provide the equivalent discovery path.

## Read-only guarantees

Writes are prevented in two independent layers:

1. The application rejects statements beginning with mutating keywords, including `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, `REPLACE`, `GRANT`, `REVOKE`, `RENAME`, `LOCK`, `UNLOCK`, `FLUSH`, `KILL`, `SHUTDOWN`, `PURGE`, `RESET`, `SET`, `LOAD`, `OPTIMIZE`, `ANALYZE`, `CALL`, `PREPARE`, `EXECUTE`, and `DEALLOCATE`.
2. The database session is made read-only: MySQL uses `SET SESSION TRANSACTION READ ONLY`; PostgreSQL uses `SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY`.

The database-level restriction is deliberately retained even when the application-level check recognises a query. It protects against statements that are syntactically unusual or not covered by a prefix check.

## Workflows

### MySQL: discover and inspect

```text
sql_query(environment="development", database="myapp", query="SHOW TABLES")
sql_query(environment="development", database="myapp", query="DESCRIBE users")
sql_query(environment="development", database="myapp", query="SELECT id, email FROM users LIMIT 20")
```

### PostgreSQL: discover through catalog views

```text
sql_query(environment="analytics", database="reporting", query="SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' LIMIT 20")
sql_query(environment="analytics", database="reporting", query="SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'users' ORDER BY ordinal_position")
```

### Explain a focused query

```text
sql_query(environment="analytics", database="reporting", query="EXPLAIN SELECT id, created_at FROM events WHERE account_id = 42 ORDER BY created_at DESC LIMIT 50")
```

## Operational guidance

- Always inspect the runtime tool description first and use an environment name shown there.
- Add `LIMIT` unless every row is genuinely required.
- Prefer explicit column names to `SELECT *`.
- Use MySQL `SHOW TABLES`/`DESCRIBE` and PostgreSQL `information_schema`/`pg_catalog` before guessing schema names.
- Keep queries focused. The connection pool permits at most three open connections per configured environment, with one idle connection retained.
- Do not rely on this interface for writes, migrations, DDL, or administrative SQL. It is intentionally an inspection interface.

For Jenkins administration and process profiling, return to the [documentation map](../README.md#documentation-map).
