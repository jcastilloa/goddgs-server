package operations

import (
	"strings"
	"testing"
)

func TestAuthenticationPagesUsePasswordManagerAutocompleteValues(t *testing.T) {
	setup := string(authPageHTML("Create dashboard access", "description", "Create account", "setup"))
	if !strings.Contains(setup, `autocomplete="new-password"`) {
		t.Errorf("setup page password autocomplete is missing: %s", setup)
	}

	login := string(authPageHTML("Welcome back", "description", "Sign in", "login"))
	if !strings.Contains(login, `autocomplete="current-password"`) {
		t.Errorf("login page password autocomplete is missing: %s", login)
	}
}
