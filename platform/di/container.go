package di

import (
	"fmt"

	sqlApp "github.com/jcastillo/gaz-mcp/mcp/application/sql"
	sqlDomain "github.com/jcastillo/gaz-mcp/mcp/domain/sql"
	"github.com/jcastillo/gaz-mcp/platform/mcp/commands"
	sqlInfra "github.com/jcastillo/gaz-mcp/platform/mcp/sql"
	mcpserver "github.com/jcastillo/gaz-mcp/platform/mcp/server"
	"github.com/jcastillo/gaz-mcp/platform/mcp/tools"
	aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"

	"github.com/sarulabs/di"
)

const OpenAIRepositoryLabel = "ai.openai.repository"

type Container struct {
	aiRepository aiDomain.AIRepository
	serviceName  string
	serviceCfg   configDomain.ServiceConfig
	environments map[string]configDomain.EnvironmentConfig
}

func New(aiRepository aiDomain.AIRepository, serviceName string, serviceCfg configDomain.ServiceConfig, environments map[string]configDomain.EnvironmentConfig) *Container {
	return &Container{
		aiRepository: aiRepository,
		serviceName:  serviceName,
		serviceCfg:   serviceCfg,
		environments: environments,
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

	serviceLabels := make([]string, 0, len(c.environments))
	for envName, envCfg := range c.environments {
		repoLabel := "mcp.sql.repo." + envName
		svcLabel := "mcp.sql.svc." + envName
		serviceLabels = append(serviceLabels, svcLabel)

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

	envNames := make([]string, 0, len(c.environments))
	for name := range c.environments {
		envNames = append(envNames, name)
	}

	defs = append(defs, di.Def{
		Name:  "mcp.sql.tool",
		Scope: di.App,
		Build: func(ctn di.Container) (interface{}, error) {
			services := make(map[string]sqlApp.Service, len(c.environments))
			for _, envName := range envNames {
				svcLabel := "mcp.sql.svc." + envName
				services[envName] = ctn.Get(svcLabel).(sqlApp.Service)
			}
			return tools.NewSQLQuery(services), nil
		},
	})

	defs = append(defs, di.Def{
		Name:  commands.RootCommandLabel,
		Scope: di.App,
		Build: func(ctn di.Container) (interface{}, error) {
			server := ctn.Get("mcp.server").(*mcpserver.Server)
			sqlTool := ctn.Get("mcp.sql.tool").(tools.SQLQuery)
			return commands.NewRunner(c.serviceName, c.serviceCfg, server, sqlTool), nil
		},
	})

	err = builder.Add(defs...)
	if err != nil {
		return nil, fmt.Errorf("register dependencies: %w", err)
	}

	container := builder.Build()
	return &container, nil
}
