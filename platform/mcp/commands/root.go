package commands

import (
	"fmt"

	mcpserver "github.com/jcastillo/gaz-mcp/platform/mcp/server"
	"github.com/jcastillo/gaz-mcp/platform/mcp/tools"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const RootCommandLabel = "mcp.root.command"

type Runner struct {
	serviceName string
	serviceCfg  configDomain.ServiceConfig
	version     string
	server      *mcpserver.Server
	sqlTool     tools.SQLQuery
}

func NewRunner(serviceName string, serviceCfg configDomain.ServiceConfig, version string, server *mcpserver.Server, sqlTool tools.SQLQuery) Runner {
	return Runner{
		serviceName: serviceName,
		serviceCfg:  serviceCfg,
		version:     version,
		server:      server,
		sqlTool:     sqlTool,
	}
}

func (r Runner) Execute() error {
	return r.newRootCommand().Execute()
}

func (r Runner) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   r.serviceName,
		Short: "MCP SQL proxy for MySQL and PostgreSQL dev databases",
		RunE:  r.runServer(),
	}

	defaultTransport := r.serviceCfg.NormalizedTransport()
	cmd.Flags().String("transport", defaultTransport, "MCP transport (supported: stdio)")

	_ = viper.BindPFlag("service.transport", cmd.Flags().Lookup("transport"))
	viper.SetDefault("service.transport", defaultTransport)
	viper.SetEnvPrefix("MCP")
	viper.AutomaticEnv()

	cmd.AddCommand(r.newVersionCommand())
	return cmd
}

func (r Runner) runServer() func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		r.server.AddTool(r.sqlTool.Definition(), r.sqlTool.Handler)

		transport := viper.GetString("service.transport")
		if transport == "" {
			transport = r.serviceCfg.NormalizedTransport()
		}

		fmt.Printf("mcp server ready: service=%s version=%s transport=%s\n", r.serviceName, r.serviceCfg.Version, transport)
		return r.server.Run(transport)
	}
}

func (r Runner) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print service version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(r.version)
		},
	}
}
