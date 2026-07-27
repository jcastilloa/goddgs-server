package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jcastilloa/goddgs-server/platform/config"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	goddgsPlatform "github.com/jcastilloa/goddgs-server/platform/goddgs"
	"github.com/jcastilloa/goddgs-server/platform/server"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
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

	containerBuilder := containerdi.New(serverCfg.Service.Version, searchService)
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
