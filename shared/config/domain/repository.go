package domain

import aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"

type Repository interface {
	MySQLConfig() MySQLConfig
	OpenAIProviderConfig() aiDomain.ProviderConfig
	ServiceConfig() ServiceConfig
}
