package server

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type Server struct {
	mcpServer *mcpserver.MCPServer
}

func New(name, version string) *Server {
	base := mcpserver.NewMCPServer(name, version, mcpserver.WithToolCapabilities(true))
	return &Server{mcpServer: base}
}

func (s *Server) AddTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	s.mcpServer.AddTool(tool, handler)
}

func (s *Server) Run(transport string) error {
	switch normalizeTransport(transport) {
	case "stdio":
		return mcpserver.ServeStdio(s.mcpServer)
	default:
		return fmt.Errorf("unsupported mcp transport %q (supported: stdio)", transport)
	}
}

func normalizeTransport(transport string) string {
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport == "" {
		return "stdio"
	}
	return transport
}
