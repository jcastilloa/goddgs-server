package main

import (
	"log"

	"github.com/jcastilloa/goddgs-server/platform/config"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	"github.com/jcastilloa/goddgs-server/platform/server"
)

func main() {
	cfgRepo, err := config.New("goddgs-server")
	if err != nil {
		log.Fatal(err)
	}

	serverCfg := cfgRepo.ServerConfig()

	containerBuilder := containerdi.New(serverCfg.Service.Version)
	container, err := containerBuilder.Build()
	if err != nil {
		log.Fatal(err)
	}

	httpServer := server.New(*container, serverCfg.Service.APIPrefix)
	log.Printf("http server listening on %s%s", serverCfg.Service.HTTPAddress(), serverCfg.Service.NormalizedAPIPrefix())

	if err := httpServer.Run(serverCfg.Service.HTTPAddress()); err != nil {
		log.Fatal(err)
	}
}
