package main

import (
	"context"
	"log"

	"github.com/jcastilloa/goddgs-server/platform/config"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	goddgsPlatform "github.com/jcastilloa/goddgs-server/platform/goddgs"
	"github.com/jcastilloa/goddgs-server/platform/server"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
)

func main() {
	cfgRepo, err := config.New("goddgs-server")
	if err != nil {
		log.Fatal(err)
	}

	serverCfg := cfgRepo.ServerConfig()
	gateway, err := goddgsPlatform.NewGatewayBuilder().Build(context.Background(), serverCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer gateway.Close()

	searchService := searchApplication.NewService(gateway.Gateway)

	containerBuilder := containerdi.New(serverCfg.Service.Version, searchService)
	container, err := containerBuilder.Build()
	if err != nil {
		log.Fatal(err)
	}

	httpServer := server.New(*container, serverCfg.Service.APIPrefix, serverCfg.AuthToken, serverCfg.RequestTimeout)
	log.Printf("http server listening on %s%s", serverCfg.Service.HTTPAddress(), serverCfg.Service.NormalizedAPIPrefix())

	if err := httpServer.Run(serverCfg.Service.HTTPAddress()); err != nil {
		log.Fatal(err)
	}
}
