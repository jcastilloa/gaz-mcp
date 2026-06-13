package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sqlApp "github.com/jcastillo/gaz-mcp/mcp/application/sql"

	"github.com/mark3labs/mcp-go/mcp"
)

type SQLQuery struct {
	service sqlApp.Service
}

func NewSQLQuery(service sqlApp.Service) SQLQuery {
	return SQLQuery{service: service}
}

func (s SQLQuery) Definition() mcp.Tool {
	return mcp.NewTool("sql_query",
		mcp.WithDescription("Execute a read-only SQL query (SELECT, SHOW, DESCRIBE, EXPLAIN) against a MySQL database. Returns JSON with columns and rows."),
		mcp.WithString("database",
			mcp.Required(),
			mcp.Description("MySQL database name to query"),
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("SQL query to execute (SELECT, SHOW, DESCRIBE, EXPLAIN only)"),
		),
	)
}

func (s SQLQuery) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database := strings.TrimSpace(mcp.ParseString(request, "database", ""))
	if database == "" {
		return mcp.NewToolResultError("database parameter is required"), nil
	}

	query := strings.TrimSpace(mcp.ParseString(request, "query", ""))
	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	result, err := s.service.ExecuteQuery(ctx, database, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("query error: %v", err)), nil
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
