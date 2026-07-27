package di

import (
	"fmt"

	helloHandler "github.com/jcastilloa/goddgs-server/platform/handlers/hello"
	systemHandler "github.com/jcastilloa/goddgs-server/platform/handlers/system"

	"github.com/sarulabs/di"
)

type Container struct {
	serviceVersion string
}

func New(serviceVersion string) *Container {
	return &Container{
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
