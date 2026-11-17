package di

import (
	"testing"

	sqlInfra "github.com/jcastillo/gaz-mcp/platform/mcp/sql"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"
)

func TestBuildSelectsPostgresRepositoryForPostgresqlEngine(t *testing.T) {
	container, err := New(
		nil,
		"gaz-mcp",
		configDomain.ServiceConfig{},
		map[string]configDomain.EnvironmentConfig{
			"repit": {Engine: "postgresql"},
		},
		nil,
		configDomain.SnapshotConfig{Enabled: false},
		configDomain.ImageGenerationConfig{},
		"test",
	).Build()
	if err != nil {
		t.Fatalf("build container: %v", err)
	}

	repository := (*container).Get("mcp.sql.repo.repit")
	if _, ok := repository.(*sqlInfra.PostgresRepository); !ok {
		t.Fatalf("repository type = %T, want *PostgresRepository", repository)
	}
}
