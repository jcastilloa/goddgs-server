package operations

import (
	_ "embed"
	"strings"
)

//go:embed assets/auth.html
var authenticationHTML string

func authPageHTML(title, description, action, mode string) []byte {
	page := strings.NewReplacer(
		"{{TITLE}}", title,
		"{{DESCRIPTION}}", description,
		"{{ACTION}}", action,
		"{{MODE}}", mode,
		"{{PASSWORD_AUTOCOMPLETE}}", passwordAutocomplete(mode),
	).Replace(authenticationHTML)
	return []byte(page)
}

func passwordAutocomplete(mode string) string {
	if mode == "setup" {
		return "new-password"
	}
	return "current-password"
}
