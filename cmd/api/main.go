package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jcastilloa/goddgs-server/platform/config"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	extractAIPlatform "github.com/jcastilloa/goddgs-server/platform/extractai"
	goddgsPlatform "github.com/jcastilloa/goddgs-server/platform/goddgs"
	searchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/search"
	openAIPlatform "github.com/jcastilloa/goddgs-server/platform/openai"
	"github.com/jcastilloa/goddgs-server/platform/server"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
	extractAIApplication "github.com/jcastilloa/goddgs-server/shared/extractai/application"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgRepo, err := config.New("goddgs-server")
	if err != nil {
		log.Print(err)
		return
	}

	serverCfg := cfgRepo.ServerConfig()
	gateway, err := goddgsPlatform.NewGatewayBuilder().Build(context.Background(), serverCfg)
	if err != nil {
		log.Print(err)
		return
	}
	defer gateway.Close()

	searchService := searchApplication.NewService(gateway.Gateway)
	var extractAIService searchHandler.ExtractAIUseCase
	if configurationError := serverCfg.AIExtractionConfigurationError(); configurationError != nil {
		extractAIService = extractAIApplication.NewUnavailableService(configurationError)
	} else {
		model, err := openAIPlatform.NewCompatibleExtractClient(serverCfg.LLM, serverCfg.ExtractAI)
		if err != nil {
			extractAIService = extractAIApplication.NewUnavailableService(err)
		} else {
			service := extractAIApplication.NewService(extractAIPlatform.NewSource(gateway.Gateway), model)
			extractAIService = service
		}
	}

	containerBuilder := containerdi.New(serverCfg.Service.Version, searchService, extractAIService)
	container, err := containerBuilder.Build()
	if err != nil {
		log.Print(err)
		return
	}
	defer (*container).Delete()

	httpServer := server.New(*container, serverCfg.Service.APIPrefix, serverCfg.Service.Version, serverCfg.AuthToken, serverCfg.RequestTimeout)
	log.Printf("http server listening on %s%s", serverCfg.Service.HTTPAddress(), serverCfg.Service.NormalizedAPIPrefix())

	if err := httpServer.Run(ctx, serverCfg.Service.HTTPAddress()); err != nil {
		log.Print(err)
	}
}
