package main

import (
	"log"

	"github.com/jcastillo/gaz-mcp/platform/config"
	containerdi "github.com/jcastillo/gaz-mcp/platform/di"
	"github.com/jcastillo/gaz-mcp/platform/mcp/commands"
	"github.com/jcastillo/gaz-mcp/platform/openai"
	"github.com/jcastillo/gaz-mcp/shared/buildinfo"
)

func main() {
	cfgRepo, err := config.New("gaz-mcp")
	if err != nil {
		log.Fatal(err)
	}

	serviceCfg := cfgRepo.ServiceConfig()
	environments := cfgRepo.Environments()
	jenkinsEnvs := cfgRepo.JenkinsEnvironments()
	snapshotCfg := cfgRepo.SnapshotConfig()
	openaiRepo := openai.NewOpenAIRepository(cfgRepo.OpenAIProviderConfig(), nil)

	containerBuilder := containerdi.New(openaiRepo, "gaz-mcp", serviceCfg, environments, jenkinsEnvs, snapshotCfg, buildinfo.NormalizedVersion())
	container, err := containerBuilder.Build()
	if err != nil {
		log.Fatal(err)
	}

	runner := (*container).Get(commands.RootCommandLabel).(commands.Runner)
	if err := runner.Execute(); err != nil {
		log.Fatal(err)
	}
}
