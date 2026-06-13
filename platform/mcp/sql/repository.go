package sql

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/sql"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(cfg configDomain.EnvironmentConfig) (domain.Repository, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)

	return &MySQLRepository{db: db}, nil
}

func (r *MySQLRepository) Query(ctx context.Context, database, query string) (domain.QueryResult, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("get connection: %w", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("set read only: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "USE "+database)
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("use database %s: %w", database, err)
	}

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("get columns: %w", err)
	}

	var results [][]string
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return domain.QueryResult{}, fmt.Errorf("scan row: %w", err)
		}

		row := make([]string, len(columns))
		for i, val := range values {
			row[i] = columnToString(val)
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return domain.QueryResult{}, fmt.Errorf("rows iteration: %w", err)
	}

	return domain.QueryResult{Columns: columns, Rows: results}, nil
}

func (r *MySQLRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func columnToString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
