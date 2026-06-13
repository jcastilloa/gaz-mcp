package sql

import (
	"context"
	"errors"
	"strings"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/sql"
)

var disallowed = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER",
	"CREATE", "TRUNCATE", "REPLACE", "GRANT", "REVOKE",
	"RENAME", "LOCK", "UNLOCK", "FLUSH",
	"KILL", "SHUTDOWN", "PURGE", "RESET", "SET ",
	"LOAD ", "OPTIMIZE", "ANALYZE",
	"CALL ", "PREPARE", "EXECUTE", "DEALLOCATE",
}

type Service struct {
	repository domain.Repository
}

func NewService(repository domain.Repository) Service {
	return Service{repository: repository}
}

func (s Service) ExecuteQuery(ctx context.Context, database, query string) (domain.QueryResult, error) {
	db := strings.TrimSpace(database)
	if db == "" {
		return domain.QueryResult{}, errors.New("database is required")
	}

	q := strings.TrimSpace(query)
	if q == "" {
		return domain.QueryResult{}, errors.New("query is empty")
	}

	upper := strings.ToUpper(q)
	for _, keyword := range disallowed {
		if strings.HasPrefix(upper, keyword) {
			return domain.QueryResult{}, errors.New("write statements are not allowed")
		}
	}

	return s.repository.Query(ctx, db, q)
}

func (s Service) Ping(ctx context.Context) error {
	return s.repository.Ping(ctx)
}
