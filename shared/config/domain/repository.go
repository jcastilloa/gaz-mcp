package domain

import aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"

type Repository interface {
	Environments() map[string]EnvironmentConfig
	OpenAIProviderConfig() aiDomain.ProviderConfig
	ServiceConfig() ServiceConfig
}
