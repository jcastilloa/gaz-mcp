package di

import (
	"fmt"

	jenkinsApp "github.com/jcastillo/gaz-mcp/mcp/application/jenkins"
	sqlApp "github.com/jcastillo/gaz-mcp/mcp/application/sql"
	jenkinsDomain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
	sqlDomain "github.com/jcastillo/gaz-mcp/mcp/domain/sql"
	"github.com/jcastillo/gaz-mcp/platform/mcp/commands"
	jenkinsInfra "github.com/jcastillo/gaz-mcp/platform/mcp/jenkins"
	mcpserver "github.com/jcastillo/gaz-mcp/platform/mcp/server"
	snapshotInfra "github.com/jcastillo/gaz-mcp/platform/mcp/snapshot"
	sqlInfra "github.com/jcastillo/gaz-mcp/platform/mcp/sql"
	"github.com/jcastillo/gaz-mcp/platform/mcp/tools"
	aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"

	"github.com/sarulabs/di"
)

const OpenAIRepositoryLabel = "ai.openai.repository"

// Container holds all wiring configuration for the DI graph.
type Container struct {
	aiRepository aiDomain.AIRepository
	serviceName  string
	serviceCfg   configDomain.ServiceConfig
	environments map[string]configDomain.EnvironmentConfig
	jenkinsEnvs  map[string]configDomain.JenkinsEnvironmentConfig
	snapshotCfg  configDomain.SnapshotConfig
	version      string
}

func New(
	aiRepository aiDomain.AIRepository,
	serviceName string,
	serviceCfg configDomain.ServiceConfig,
	environments map[string]configDomain.EnvironmentConfig,
	jenkinsEnvs map[string]configDomain.JenkinsEnvironmentConfig,
	snapshotCfg configDomain.SnapshotConfig,
	version string,
) *Container {
	return &Container{
		aiRepository: aiRepository,
		serviceName:  serviceName,
		serviceCfg:   serviceCfg,
		environments: environments,
		jenkinsEnvs:  jenkinsEnvs,
		snapshotCfg:  snapshotCfg,
		version:      version,
	}
}

func (c *Container) Build() (*di.Container, error) {
	builder, err := di.NewBuilder()
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	defs := []di.Def{
		{
			Name:  OpenAIRepositoryLabel,
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return c.aiRepository, nil
			},
		},
		{
			Name:  "mcp.server",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return mcpserver.New(c.serviceName, c.serviceCfg.Version), nil
			},
		},
	}

	// --- SQL services ---
	sqlEnvNames := make([]string, 0, len(c.environments))
	for envName, envCfg := range c.environments {
		repoLabel := "mcp.sql.repo." + envName
		svcLabel := "mcp.sql.svc." + envName
		sqlEnvNames = append(sqlEnvNames, envName)

		defs = append(defs, di.Def{
			Name:  repoLabel,
			Scope: di.App,
			Build: func(cfg configDomain.EnvironmentConfig) func(ctn di.Container) (interface{}, error) {
				return func(ctn di.Container) (interface{}, error) {
					if cfg.Engine == "postgres" {
						return sqlInfra.NewPostgresRepository(cfg)
					}
					return sqlInfra.NewMySQLRepository(cfg)
				}
			}(envCfg),
		})

		defs = append(defs, di.Def{
			Name:  svcLabel,
			Scope: di.App,
			Build: func(label string) func(ctn di.Container) (interface{}, error) {
				return func(ctn di.Container) (interface{}, error) {
					repo := ctn.Get(label).(sqlDomain.Repository)
					return sqlApp.NewService(repo), nil
				}
			}(repoLabel),
		})
	}

	defs = append(defs, di.Def{
		Name:  "mcp.sql.tool",
		Scope: di.App,
		Build: func(ctn di.Container) (interface{}, error) {
			services := make(map[string]sqlApp.Service, len(c.environments))
			for _, envName := range sqlEnvNames {
				svcLabel := "mcp.sql.svc." + envName
				services[envName] = ctn.Get(svcLabel).(sqlApp.Service)
			}
			return tools.NewSQLQuery(services), nil
		},
	})

	// --- Snapshot repository (shared across all Jenkins environments) ---
	defs = append(defs, di.Def{
		Name:  "mcp.snapshot.repo",
		Scope: di.App,
		Build: func(ctn di.Container) (interface{}, error) {
			if !c.snapshotCfg.Enabled {
				return &jenkinsApp.NoopSnapshotRepository{}, nil
			}
			return snapshotInfra.NewRepository(c.snapshotCfg.DBPath)
		},
		Close: func(obj interface{}) error {
			if closer, ok := obj.(interface{ Close() error }); ok {
				return closer.Close()
			}
			return nil
		},
	})

	// --- Jenkins services (one per environment) ---
	jenkinsEnvNames := make([]string, 0, len(c.jenkinsEnvs))
	for envName, envCfg := range c.jenkinsEnvs {
		repoLabel := "mcp.jenkins.repo." + envName
		svcLabel := "mcp.jenkins.svc." + envName
		jenkinsEnvNames = append(jenkinsEnvNames, envName)

		defs = append(defs, di.Def{
			Name:  repoLabel,
			Scope: di.App,
			Build: func(cfg configDomain.JenkinsEnvironmentConfig) func(ctn di.Container) (interface{}, error) {
				return func(ctn di.Container) (interface{}, error) {
					return jenkinsInfra.NewRepository(cfg)
				}
			}(envCfg),
		})

		defs = append(defs, di.Def{
			Name:  svcLabel,
			Scope: di.App,
			Build: func(repoLbl string) func(ctn di.Container) (interface{}, error) {
				return func(ctn di.Container) (interface{}, error) {
					repo := ctn.Get(repoLbl).(*jenkinsInfra.Repository)
					snapRepo := ctn.Get("mcp.snapshot.repo").(jenkinsDomain.SnapshotRepository)
					return jenkinsApp.NewService(repo, snapRepo, c.snapshotCfg.MaxVersions), nil
				}
			}(repoLabel),
		})
	}

	// --- Jenkins tools ---
	defs = append(defs, di.Def{
		Name:  "mcp.jenkins.tools",
		Scope: di.App,
		Build: func(ctn di.Container) (interface{}, error) {
			services := make(map[string]*jenkinsApp.Service, len(c.jenkinsEnvs))
			for _, envName := range jenkinsEnvNames {
				svcLabel := "mcp.jenkins.svc." + envName
				services[envName] = ctn.Get(svcLabel).(*jenkinsApp.Service)
			}
			return commands.JenkinsTools{
				Read:     tools.NewJenkinsRead(services),
				Write:    tools.NewJenkinsWrite(services),
				Snapshot: tools.NewJenkinsSnapshot(services),
			}, nil
		},
	})

	// --- Root command ---
	defs = append(defs, di.Def{
		Name:  commands.RootCommandLabel,
		Scope: di.App,
		Build: func(ctn di.Container) (interface{}, error) {
			server := ctn.Get("mcp.server").(*mcpserver.Server)
			sqlTool := ctn.Get("mcp.sql.tool").(tools.SQLQuery)

			var jenkinsTools commands.JenkinsTools
			if len(c.jenkinsEnvs) > 0 {
				jenkinsTools = ctn.Get("mcp.jenkins.tools").(commands.JenkinsTools)
			}

			return commands.NewRunner(
				c.serviceName, c.serviceCfg, c.version,
				server, sqlTool, jenkinsTools,
			), nil
		},
	})

	if err := builder.Add(defs...); err != nil {
		return nil, fmt.Errorf("register dependencies: %w", err)
	}

	container := builder.Build()
	return &container, nil
}
