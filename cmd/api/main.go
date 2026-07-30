package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operationsDomain "github.com/jcastilloa/goddgs-server/operations/domain"
	chromePlatform "github.com/jcastilloa/goddgs-server/platform/chrome"
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
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
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
	chromeExecutable := chromePlatform.NewExecutableLocator(ctx, serverCfg.Chrome.ExecutablePath, func(path string) {
		if err := cfgRepo.PersistChromeExecutablePath(ctx, path); err != nil {
			log.Printf("persist discovered Chrome executable: %v", err)
		}
	})
	operationsStore, err := operationsSQLite.Open(ctx, operationsSQLite.Config{DatabasePath: serverCfg.Operations.DatabasePath})
	if err != nil {
		log.Print(err)
		return
	}
	defer closeOperationsStore(operationsStore)
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
	defer closeGateway(gateway)
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

	loader, chromeManager := htmlLoader(serverCfg, gateway, eventRecorder, chromeExecutable)
	defer closeChromeManager(chromeManager)
	searchService := searchApplication.NewService(gateway.Gateway, loader)
	var extractAIService searchHandler.ExtractAIUseCase
	if configurationError := serverCfg.AIExtractionConfigurationError(); configurationError != nil {
		extractAIService = extractAIApplication.NewUnavailableService(configurationError)
	} else {
		model, err := openAIPlatform.NewCompatibleExtractClient(serverCfg.LLM, serverCfg.ExtractAI)
		if err != nil {
			extractAIService = extractAIApplication.NewUnavailableService(err)
		} else {
			source := operationsApplication.NewSourceRecorder(extractAIPlatform.NewSource(searchService), eventRecorder)
			recordedModel := operationsApplication.NewCompletionModelRecorder(model, eventRecorder, "extract_ai", "openai-compatible", serverCfg.ExtractAI.Model)
			service := extractAIApplication.NewService(source, recordedModel)
			extractAIService = service
		}
	}

	researchService := newResearchUseCase(serverCfg, searchService, extractAIService, eventRecorder)

	dashboardService := operationsApplication.NewDashboardService(operationsStore)
	dashboardAuthService := operationsApplication.NewDashboardAuthService(operationsStore, operationsApplication.DashboardAuthConfig{SessionTTL: serverCfg.Operations.DashboardAuth.SessionTTL})
	containerBuilder := containerdi.New(serverCfg.Service.Version, searchService, extractAIService, researchService, dashboardService, containerdi.WithDashboardAuth(dashboardAuthService, serverCfg.Operations.DashboardAuth.CookieSecure))
	container, err := containerBuilder.Build()
	if err != nil {
		log.Print(err)
		return
	}
	defer (*container).Delete()

	httpServer := server.NewWithRecorderAndDashboardAuth(*container, serverCfg.Service.APIPrefix, serverCfg.Service.Version, serverCfg.AuthToken, serverCfg.RequestTimeout, serverCfg.Research.Timeout, eventRecorder, dashboardAuthService, serverCfg.Operations.DashboardAuth.CookieSecure)
	log.Printf("http server listening on %s%s", serverCfg.Service.HTTPAddress(), serverCfg.Service.NormalizedAPIPrefix())

	if err := httpServer.Run(ctx, serverCfg.Service.HTTPAddress()); err != nil {
		log.Print(err)
	}
}

func htmlLoader(serverCfg configDomain.ServerConfig, gateway *goddgsPlatform.ManagedGateway, recorder operationsApplication.EventRecorder, executable *chromePlatform.ExecutableLocator) (searchApplication.HTMLLoader, io.Closer) {
	if !serverCfg.Chrome.Enabled {
		return nil, nil
	}
	manager := chromePlatform.NewManager(chromePlatform.ManagerConfig{
		MaxBrowsers:        serverCfg.Chrome.MaxBrowsers,
		MaxPagesPerBrowser: serverCfg.Chrome.MaxPagesPerBrowser,
		IdleTimeout:        serverCfg.Chrome.IdleTimeout,
		Factory:            chromePlatform.NewChromedpFactoryWithLocator(executable),
	})
	return chromePlatform.NewLoader(gateway.ProxySelector(), manager, serverCfg.Chrome.Timeout, nil, recorder), manager
}

func closeChromeManager(manager io.Closer) {
	if manager == nil {
		return
	}
	if err := manager.Close(); err != nil {
		log.Print("close Chrome manager failed")
	}
}

func closeGateway(gateway io.Closer) {
	if err := gateway.Close(); err != nil {
		log.Printf("close gateway: %v", err)
	}
}

func closeOperationsStore(store io.Closer) {
	if err := store.Close(); err != nil {
		log.Printf("close operations storage: %v", err)
	}
}

func newResearchUseCase(serverCfg configDomain.ServerConfig, searcher researchApplication.Searcher, extractor researchApplication.Extractor, eventRecorder operationsApplication.EventRecorder) researchHandler.UseCase {
	if configurationError := serverCfg.ResearchConfigurationError(); configurationError != nil {
		return researchApplication.NewUnavailableService(configurationError)
	}
	queryModel, queryError := openAIPlatform.NewCompatibleResearchClient(serverCfg.LLM, serverCfg.Research.QueryAI)
	selectionModel, selectionError := openAIPlatform.NewCompatibleResearchClient(serverCfg.LLM, serverCfg.Research.SelectionAI)
	reportModel, reportError := openAIPlatform.NewCompatibleResearchClient(serverCfg.LLM, serverCfg.Research.ReportAI)
	if queryError != nil {
		return researchApplication.NewUnavailableService(queryError)
	}
	if selectionError != nil {
		return researchApplication.NewUnavailableService(selectionError)
	}
	if reportError != nil {
		return researchApplication.NewUnavailableService(reportError)
	}

	recordedQueryModel := operationsApplication.NewCompletionModelRecorder(queryModel, eventRecorder, operationsDomain.StepLLMPlanning, "openai-compatible", serverCfg.Research.QueryAI.Model)
	recordedSelectionModel := operationsApplication.NewCompletionModelRecorder(selectionModel, eventRecorder, operationsDomain.StepLLMSelection, "openai-compatible", serverCfg.Research.SelectionAI.Model)
	recordedReportModel := operationsApplication.NewCompletionModelRecorder(reportModel, eventRecorder, operationsDomain.StepLLMReport, "openai-compatible", serverCfg.Research.ReportAI.Model)
	return researchApplication.NewService(
		researchApplication.NewLLMPlanner(recordedQueryModel, serverCfg.Research.QueryAI.Retries),
		researchApplication.NewLLMSelector(recordedSelectionModel),
		searcher,
		extractor,
		researchApplication.NewLLMReporter(recordedReportModel),
		researchApplication.Limits{
			MaxSelectionCandidates:   serverCfg.Research.MaxSelectionCandidates,
			MaxSelectedSources:       serverCfg.Research.MaxSelectedSources,
			MaxConcurrentExtractions: serverCfg.Research.MaxConcurrentExtractions,
		},
		eventRecorder,
	)
}
