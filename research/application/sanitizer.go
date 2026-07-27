package application

import extractAIApplication "github.com/jcastilloa/goddgs-server/shared/extractai/application"

func sanitizeReportHTML(content string) (string, error) {
	return extractAIApplication.SanitizeHTML(content)
}
