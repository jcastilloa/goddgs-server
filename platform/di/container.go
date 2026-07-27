package di

import (
	"fmt"

	helloHandler "github.com/jcastilloa/goddgs-server/platform/handlers/hello"
	systemHandler "github.com/jcastilloa/goddgs-server/platform/handlers/system"
	aiDomain "github.com/jcastilloa/goddgs-server/shared/ai/domain"

	"github.com/sarulabs/di"
)

const OpenAIRepositoryLabel = "ai.openai.repository"

type Container struct {
	aiRepository   aiDomain.AIRepository
	serviceVersion string
}

func New(aiRepository aiDomain.AIRepository, serviceVersion string) *Container {
	return &Container{
		aiRepository:   aiRepository,
		serviceVersion: serviceVersion,
	}
}

func (c *Container) Build() (*di.Container, error) {
	builder, err := di.NewBuilder()
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	err = builder.Add(
		di.Def{
			Name:  OpenAIRepositoryLabel,
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return c.aiRepository, nil
			},
		},
		di.Def{
			Name:  helloHandler.GetHelloHandlerLabel,
			Scope: di.App,
			Build: func(ctn di.Container) (interface{}, error) {
				return helloHandler.NewGet(), nil
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
