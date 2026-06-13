package sql

import "context"

type QueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type Repository interface {
	Query(ctx context.Context, database, query string) (QueryResult, error)
	Ping(ctx context.Context) error
}
