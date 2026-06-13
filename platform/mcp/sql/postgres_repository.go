package sql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/sql"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"

	_ "github.com/lib/pq"
)

type PostgresRepository struct {
	cfg configDomain.EnvironmentConfig
	dbs map[string]*sql.DB
	mu  sync.RWMutex
}

func NewPostgresRepository(cfg configDomain.EnvironmentConfig) (domain.Repository, error) {
	return &PostgresRepository{
		cfg: cfg,
		dbs: make(map[string]*sql.DB),
	}, nil
}

func (r *PostgresRepository) getDB(database string) (*sql.DB, error) {
	r.mu.RLock()
	db, ok := r.dbs[database]
	r.mu.RUnlock()
	if ok {
		return db, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if db, ok := r.dbs[database]; ok {
		return db, nil
	}

	db, err := sql.Open("postgres", r.cfg.PostgresDSN(database))
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)

	r.dbs[database] = db
	return db, nil
}

func (r *PostgresRepository) Query(ctx context.Context, database, query string) (domain.QueryResult, error) {
	db, err := r.getDB(database)
	if err != nil {
		return domain.QueryResult{}, err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("get connection: %w", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY")
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("set read only: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return domain.QueryResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

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

func (r *PostgresRepository) Ping(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, db := range r.dbs {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
	}
	return nil
}
