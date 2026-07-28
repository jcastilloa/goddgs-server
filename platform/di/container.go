package di

import (
	"fmt"

	operationsHandler "github.com/jcastilloa/goddgs-server/platform/handlers/operations"
	researchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/research"
	searchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/search"
	systemHandler "github.com/jcastilloa/goddgs-server/platform/handlers/system"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"

	"github.com/sarulabs/di"
)

type Container struct {
	serviceVersion string
	searchService  searchApplication.Service
	extractAI      searchHandler.ExtractAIUseCase
	research       researchHandler.UseCase
	dashboard      operationsHandler.DashboardUseCase
}

func New(serviceVersion string, searchService searchApplication.Service, extractAI searchHandler.ExtractAIUseCase, research researchHandler.UseCase, dashboard operationsHandler.DashboardUseCase) *Container {
	return &Container{
		serviceVersion: serviceVersion,
		searchService:  searchService,
		extractAI:      extractAI,
		research:       research,
		dashboard:      dashboard,
	}
}

func (c *Container) Build() (*di.Container, error) {
	if c.dashboard == nil {
		return nil, fmt.Errorf("dashboard use case is required")
	}
	builder, err := di.NewBuilder()
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	err = builder.Add(
		di.Def{
			Name:  operationsHandler.DashboardHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return operationsHandler.NewDashboard(), nil
			},
		},
		di.Def{
			Name:  operationsHandler.SummaryHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return operationsHandler.NewSummary(c.dashboard), nil
			},
		},
		di.Def{
			Name:  operationsHandler.TimeSeriesHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return operationsHandler.NewTimeSeries(c.dashboard), nil
			},
		},
		di.Def{
			Name:  operationsHandler.ListHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return operationsHandler.NewList(c.dashboard), nil
			},
		},
		di.Def{
			Name:  operationsHandler.DetailHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return operationsHandler.NewDetail(c.dashboard), nil
			},
		},
		di.Def{
			Name:  operationsHandler.ProxiesHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return operationsHandler.NewProxies(c.dashboard), nil
			},
		},
		di.Def{
			Name:  searchHandler.GetTextHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return searchHandler.NewGet(searchDomain.CategoryText, c.searchService), nil
			},
		},
		di.Def{
			Name:  researchHandler.PostHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return researchHandler.NewPost(c.research), nil
			},
		},
		di.Def{
			Name:  searchHandler.GetImagesHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return searchHandler.NewGet(searchDomain.CategoryImages, c.searchService), nil
			},
		},
		di.Def{
			Name:  searchHandler.GetNewsHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return searchHandler.NewGet(searchDomain.CategoryNews, c.searchService), nil
			},
		},
		di.Def{
			Name:  searchHandler.GetVideosHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return searchHandler.NewGet(searchDomain.CategoryVideos, c.searchService), nil
			},
		},
		di.Def{
			Name:  searchHandler.GetBooksHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return searchHandler.NewGet(searchDomain.CategoryBooks, c.searchService), nil
			},
		},
		di.Def{
			Name:  searchHandler.GetExtractHandlerLabel,
			Scope: di.App,
			Build: func(di.Container) (interface{}, error) {
				return searchHandler.NewExtract(c.searchService, c.extractAI), nil
			},
		},
		di.Def{
			Name:  systemHandler.GetVersionHandlerLabel,
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return systemHandler.NewGetVersion(c.serviceVersion), nil
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("register dependencies: %w", err)
	}

	container := builder.Build()
	return &container, nil
}
