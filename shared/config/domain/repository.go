package domain

import aiDomain "github.com/jcastilloa/goddgs-server/shared/ai/domain"

type Repository interface {
	OpenAIProviderConfig() aiDomain.ProviderConfig
	ServiceConfig() ServiceConfig
}
