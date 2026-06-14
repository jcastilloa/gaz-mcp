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

// JenkinsTools bundles the three Jenkins tool groups for injection.
type JenkinsTools struct {
	Read     tools.JenkinsRead
	Write    tools.JenkinsWrite
	Snapshot tools.JenkinsSnapshot
}

// Runner wires the cobra command tree and MCP server.
type Runner struct {
	serviceName  string
	serviceCfg   configDomain.ServiceConfig
	version      string
	server       *mcpserver.Server
	sqlTool      tools.SQLQuery
	jenkinsTools JenkinsTools
	hasJenkins   bool
}

func NewRunner(
	serviceName string,
	serviceCfg configDomain.ServiceConfig,
	version string,
	server *mcpserver.Server,
	sqlTool tools.SQLQuery,
	jenkinsTools JenkinsTools,
) Runner {
	// hasJenkins is true when at least one Jenkins environment is configured.
	// We detect this by checking whether the Read tool has any services.
	hasJenkins := len(jenkinsTools.Read.Services()) > 0
	return Runner{
		serviceName:  serviceName,
		serviceCfg:   serviceCfg,
		version:      version,
		server:       server,
		sqlTool:      sqlTool,
		jenkinsTools: jenkinsTools,
		hasJenkins:   hasJenkins,
	}
}

func (r Runner) Execute() error {
	return r.newRootCommand().Execute()
}

func (r Runner) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   r.serviceName,
		Short: "MCP proxy for MySQL, PostgreSQL and Jenkins",
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
		// --- SQL tool ---
		r.server.AddTool(r.sqlTool.Definition(), r.sqlTool.Handler)

		// --- Jenkins tools (only when environments are configured) ---
		if r.hasJenkins {
			jRead := r.jenkinsTools.Read
			jWrite := r.jenkinsTools.Write
			jSnap := r.jenkinsTools.Snapshot

			// Read tools
			r.server.AddTool(jRead.InfoDefinition(), jRead.InfoHandler)
			r.server.AddTool(jRead.JobListDefinition(), jRead.JobListHandler)
			r.server.AddTool(jRead.JobGetDefinition(), jRead.JobGetHandler)
			r.server.AddTool(jRead.JobConfigDefinition(), jRead.JobConfigHandler)
			r.server.AddTool(jRead.BuildInfoDefinition(), jRead.BuildInfoHandler)
			r.server.AddTool(jRead.BuildLogDefinition(), jRead.BuildLogHandler)
			r.server.AddTool(jRead.BuildArtifactsDefinition(), jRead.BuildArtifactsHandler)
			r.server.AddTool(jRead.NodeListDefinition(), jRead.NodeListHandler)
			r.server.AddTool(jRead.QueueListDefinition(), jRead.QueueListHandler)
			r.server.AddTool(jRead.PluginListDefinition(), jRead.PluginListHandler)
			r.server.AddTool(jRead.ViewListDefinition(), jRead.ViewListHandler)
			r.server.AddTool(jRead.CredentialListDefinition(), jRead.CredentialListHandler)

			// Write/execute tools
			r.server.AddTool(jWrite.JobSetConfigDefinition(), jWrite.JobSetConfigHandler)
			r.server.AddTool(jWrite.JobCreateDefinition(), jWrite.JobCreateHandler)
			r.server.AddTool(jWrite.JobCopyDefinition(), jWrite.JobCopyHandler)
			r.server.AddTool(jWrite.JobDeleteDefinition(), jWrite.JobDeleteHandler)
			r.server.AddTool(jWrite.JobEnableDefinition(), jWrite.JobEnableHandler)
			r.server.AddTool(jWrite.JobDisableDefinition(), jWrite.JobDisableHandler)
			r.server.AddTool(jWrite.JobBuildDefinition(), jWrite.JobBuildHandler)
			r.server.AddTool(jWrite.BuildStopDefinition(), jWrite.BuildStopHandler)
			r.server.AddTool(jWrite.QueueCancelDefinition(), jWrite.QueueCancelHandler)
			r.server.AddTool(jWrite.NodeEnableDefinition(), jWrite.NodeEnableHandler)
			r.server.AddTool(jWrite.NodeDisableDefinition(), jWrite.NodeDisableHandler)
			r.server.AddTool(jWrite.ScriptConsoleDefinition(), jWrite.ScriptConsoleHandler)
			r.server.AddTool(jWrite.CredentialCreateDefinition(), jWrite.CredentialCreateHandler)
			r.server.AddTool(jWrite.CredentialDeleteDefinition(), jWrite.CredentialDeleteHandler)
			r.server.AddTool(jWrite.ViewCreateDefinition(), jWrite.ViewCreateHandler)
			r.server.AddTool(jWrite.ViewDeleteDefinition(), jWrite.ViewDeleteHandler)

			// Snapshot tools
			r.server.AddTool(jSnap.ListDefinition(), jSnap.ListHandler)
			r.server.AddTool(jSnap.GetDefinition(), jSnap.GetHandler)
			r.server.AddTool(jSnap.DiffDefinition(), jSnap.DiffHandler)
			r.server.AddTool(jSnap.RestoreDefinition(), jSnap.RestoreHandler)
			r.server.AddTool(jSnap.PruneDefinition(), jSnap.PruneHandler)
		}

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
