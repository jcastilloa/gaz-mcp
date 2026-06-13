package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sqlApp "github.com/jcastillo/gaz-mcp/mcp/application/sql"

	"github.com/mark3labs/mcp-go/mcp"
)

type SQLQuery struct {
	services map[string]sqlApp.Service
}

func NewSQLQuery(services map[string]sqlApp.Service) SQLQuery {
	return SQLQuery{services: services}
}

func (s SQLQuery) Definition() mcp.Tool {
	envNames := make([]string, 0, len(s.services))
	for name := range s.services {
		envNames = append(envNames, name)
	}
	sort.Strings(envNames)

	return mcp.NewTool("sql_query",
		mcp.WithDescription(fmt.Sprintf(
			"Execute a read-only SQL query. Available environments: %s. Returns JSON with columns and rows.",
			strings.Join(envNames, ", "),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Environment name: %s", strings.Join(envNames, ", "))),
		),
		mcp.WithString("database",
			mcp.Required(),
			mcp.Description("Database name to query"),
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("SQL query to execute (SELECT, SHOW, DESCRIBE, EXPLAIN)"),
		),
	)
}

func (s SQLQuery) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := strings.TrimSpace(mcp.ParseString(request, "environment", ""))
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}

	svc, ok := s.services[env]
	if !ok {
		names := make([]string, 0, len(s.services))
		for name := range s.services {
			names = append(names, name)
		}
		sort.Strings(names)
		return mcp.NewToolResultError(fmt.Sprintf("unknown environment %q, available: %s", env, strings.Join(names, ", "))), nil
	}

	database := strings.TrimSpace(mcp.ParseString(request, "database", ""))
	if database == "" {
		return mcp.NewToolResultError("database parameter is required"), nil
	}

	query := strings.TrimSpace(mcp.ParseString(request, "query", ""))
	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	result, err := svc.ExecuteQuery(ctx, database, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("query error: %v", err)), nil
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
