package domain

import aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"

type Repository interface {
	MySQLConfig() MySQLConfig
	PostgresConfig() PostgresConfig
	OpenAIProviderConfig() aiDomain.ProviderConfig
	ServiceConfig() ServiceConfig
}
