package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	"github.com/jcastilloa/goddgs-server/platform/config"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	extractAIPlatform "github.com/jcastilloa/goddgs-server/platform/extractai"
	goddgsPlatform "github.com/jcastilloa/goddgs-server/platform/goddgs"
	researchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/research"
	searchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/search"
	openAIPlatform "github.com/jcastilloa/goddgs-server/platform/openai"
	operationsProbe "github.com/jcastilloa/goddgs-server/platform/operations/probe"
	operationsSQLite "github.com/jcastilloa/goddgs-server/platform/operations/sqlite"
	"github.com/jcastilloa/goddgs-server/platform/server"
	researchApplication "github.com/jcastilloa/goddgs-server/research/application"
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
	operationsStore, err := operationsSQLite.Open(ctx, operationsSQLite.Config{DatabasePath: serverCfg.Operations.DatabasePath})
	if err != nil {
		log.Print(err)
		return
	}
	defer func() {
		if err := operationsStore.Close(); err != nil {
			log.Printf("close operations storage: %v", err)
		}
	}()
	if err := operationsStore.DeleteExpired(ctx, time.Now().Add(-serverCfg.Operations.Retention)); err != nil {
		log.Printf("clean operations storage: %v", err)
		return
	}
	retentionWorker := operationsApplication.StartRetentionWorker(ctx, operationsStore, operationsApplication.RetentionWorkerConfig{
		Retention:     serverCfg.Operations.Retention,
		CheckInterval: time.Hour,
		ReportError: func(err error) {
			log.Printf("clean operations storage: %v", err)
		},
	})
	defer retentionWorker.Stop()
	eventRecorder := operationsApplication.NewEventRecorder(operationsStore, time.Now, nil)

	gateway, err := goddgsPlatform.NewGatewayBuilder().Build(ctx, serverCfg, eventRecorder)
	if err != nil {
		log.Print(err)
		return
	}
	defer gateway.Close()
	if serverCfg.Operations.Probe.Enabled {
		probeSupervisor := gateway.StartHealthProbes(ctx, goddgsPlatform.HealthProbeConfig{
			Interval:         serverCfg.Operations.Probe.Interval,
			URL:              serverCfg.Operations.Probe.URL,
			SuccessThreshold: serverCfg.Operations.Probe.SuccessThreshold,
			FailureThreshold: serverCfg.Operations.Probe.FailureThreshold,
			Store:            operationsStore,
			Prober:           operationsProbe.NewHTTPProber(serverCfg.Operations.Probe.Timeout),
			Now:              time.Now,
			ReportError: func(err error) {
				log.Printf("update proxy probe health: %v", err)
			},
		})
		defer probeSupervisor.Stop()
	}

	searchService := searchApplication.NewService(gateway.Gateway)
	var extractAIService searchHandler.ExtractAIUseCase
	if configurationError := serverCfg.AIExtractionConfigurationError(); configurationError != nil {
		extractAIService = extractAIApplication.NewUnavailableService(configurationError)
	} else {
		model, err := openAIPlatform.NewCompatibleExtractClient(serverCfg.LLM, serverCfg.ExtractAI)
		if err != nil {
			extractAIService = extractAIApplication.NewUnavailableService(err)
		} else {
			source := operationsApplication.NewSourceRecorder(extractAIPlatform.NewSource(gateway.Gateway), eventRecorder)
			recordedModel := operationsApplication.NewCompletionModelRecorder(model, eventRecorder, "extract_ai", "openai-compatible", serverCfg.ExtractAI.Model)
			service := extractAIApplication.NewService(source, recordedModel)
			extractAIService = service
		}
	}

	var researchService researchHandler.UseCase
	if configurationError := serverCfg.ResearchConfigurationError(); configurationError != nil {
		researchService = researchApplication.NewUnavailableService(configurationError)
	} else {
		queryModel, queryError := openAIPlatform.NewCompatibleResearchClient(serverCfg.LLM, serverCfg.Research.QueryAI)
		reportModel, reportError := openAIPlatform.NewCompatibleResearchClient(serverCfg.LLM, serverCfg.Research.ReportAI)
		if queryError != nil {
			researchService = researchApplication.NewUnavailableService(queryError)
		} else if reportError != nil {
			researchService = researchApplication.NewUnavailableService(reportError)
		} else {
			recordedQueryModel := operationsApplication.NewCompletionModelRecorder(queryModel, eventRecorder, "llm_planning", "openai-compatible", serverCfg.Research.QueryAI.Model)
			recordedReportModel := operationsApplication.NewCompletionModelRecorder(reportModel, eventRecorder, "llm_report", "openai-compatible", serverCfg.Research.ReportAI.Model)
			researchService = researchApplication.NewService(
				researchApplication.NewLLMPlanner(recordedQueryModel, serverCfg.Research.QueryAI.Retries),
				searchService,
				extractAIService,
				researchApplication.NewLLMReporter(recordedReportModel),
				serverCfg.Research.MaxConcurrentExtractions,
				eventRecorder,
			)
		}
	}

	dashboardService := operationsApplication.NewDashboardService(operationsStore)
	containerBuilder := containerdi.New(serverCfg.Service.Version, searchService, extractAIService, researchService, dashboardService)
	container, err := containerBuilder.Build()
	if err != nil {
		log.Print(err)
		return
	}
	defer (*container).Delete()

	httpServer := server.NewWithRecorder(*container, serverCfg.Service.APIPrefix, serverCfg.Service.Version, serverCfg.AuthToken, serverCfg.RequestTimeout, serverCfg.Research.Timeout, eventRecorder)
	log.Printf("http server listening on %s%s", serverCfg.Service.HTTPAddress(), serverCfg.Service.NormalizedAPIPrefix())

	if err := httpServer.Run(ctx, serverCfg.Service.HTTPAddress()); err != nil {
		log.Print(err)
	}
}
