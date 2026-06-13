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
	mysqlCfg     configDomain.MySQLConfig
	postgresCfg  configDomain.PostgresConfig
}

func New(aiRepository aiDomain.AIRepository, serviceName string, serviceCfg configDomain.ServiceConfig, mysqlCfg configDomain.MySQLConfig, postgresCfg configDomain.PostgresConfig) *Container {
	return &Container{
		aiRepository: aiRepository,
		serviceName:  serviceName,
		serviceCfg:   serviceCfg,
		mysqlCfg:     mysqlCfg,
		postgresCfg:  postgresCfg,
	}
}

func (c *Container) Build() (*di.Container, error) {
	builder, err := di.NewBuilder()
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	err = builder.Add(
		di.Def{
			Name:  OpenAIRepositoryLabel,
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return c.aiRepository, nil
			},
		},
		di.Def{
			Name:  "mcp.sql.mysql.repository",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return sqlInfra.NewMySQLRepository(c.mysqlCfg)
			},
		},
		di.Def{
			Name:  "mcp.sql.postgres.repository",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return sqlInfra.NewPostgresRepository(c.postgresCfg)
			},
		},
		di.Def{
			Name:  "mcp.sql.mysql.service",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				repo := ctn.Get("mcp.sql.mysql.repository").(sqlDomain.Repository)
				return sqlApp.NewService(repo), nil
			},
		},
		di.Def{
			Name:  "mcp.sql.postgres.service",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				repo := ctn.Get("mcp.sql.postgres.repository").(sqlDomain.Repository)
				return sqlApp.NewService(repo), nil
			},
		},
		di.Def{
			Name:  "mcp.sql.tool",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				mysqlSvc := ctn.Get("mcp.sql.mysql.service").(sqlApp.Service)
				postgresSvc := ctn.Get("mcp.sql.postgres.service").(sqlApp.Service)
				return tools.NewSQLQuery(mysqlSvc, postgresSvc), nil
			},
		},
		di.Def{
			Name:  "mcp.server",
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return mcpserver.New(c.serviceName, c.serviceCfg.Version), nil
			},
		},
		di.Def{
			Name:  commands.RootCommandLabel,
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				server := ctn.Get("mcp.server").(*mcpserver.Server)
				sqlTool := ctn.Get("mcp.sql.tool").(tools.SQLQuery)
				return commands.NewRunner(c.serviceName, c.serviceCfg, server, sqlTool), nil
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("register dependencies: %w", err)
	}

	container := builder.Build()
	return &container, nil
}
