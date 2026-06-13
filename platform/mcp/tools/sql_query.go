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
	mysqlService    sqlApp.Service
	postgresService sqlApp.Service
}

func NewSQLQuery(mysqlService, postgresService sqlApp.Service) SQLQuery {
	return SQLQuery{mysqlService: mysqlService, postgresService: postgresService}
}

func (s SQLQuery) Definition() mcp.Tool {
	return mcp.NewTool("sql_query",
		mcp.WithDescription("Execute a read-only SQL query against MySQL or PostgreSQL. Returns JSON with columns and rows."),
		mcp.WithString("type",
			mcp.Description("Database type: mysql or postgres (default: mysql)"),
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
	dbType := strings.ToLower(strings.TrimSpace(mcp.ParseString(request, "type", "mysql")))
	if dbType != "mysql" && dbType != "postgres" {
		return mcp.NewToolResultError("type must be 'mysql' or 'postgres'"), nil
	}

	database := strings.TrimSpace(mcp.ParseString(request, "database", ""))
	if database == "" {
		return mcp.NewToolResultError("database parameter is required"), nil
	}

	query := strings.TrimSpace(mcp.ParseString(request, "query", ""))
	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	svc := s.mysqlService
	if dbType == "postgres" {
		svc = s.postgresService
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
