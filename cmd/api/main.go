package main

import (
	"log"

	"github.com/jcastilloa/goddgs-server/platform/config"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	"github.com/jcastilloa/goddgs-server/platform/openai"
	"github.com/jcastilloa/goddgs-server/platform/server"
)

func main() {
	cfgRepo, err := config.New("goddgs-server")
	if err != nil {
		log.Fatal(err)
	}

	serviceCfg := cfgRepo.ServiceConfig()
	openaiCfg := cfgRepo.OpenAIProviderConfig()
	openaiRepo := openai.NewOpenAIRepository(openaiCfg, nil)

	containerBuilder := containerdi.New(openaiRepo, serviceCfg.Version)
	container, err := containerBuilder.Build()
	if err != nil {
		log.Fatal(err)
	}

	httpServer := server.New(*container, serviceCfg.APIPrefix)
	log.Printf("http server listening on %s%s", serviceCfg.HTTPAddress(), serviceCfg.NormalizedAPIPrefix())

	if err := httpServer.Run(serviceCfg.HTTPAddress()); err != nil {
		log.Fatal(err)
	}
}
