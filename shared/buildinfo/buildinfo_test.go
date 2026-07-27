package buildinfo

import "testing"

func TestCurrentVersionUsesBuildVersionOrDefault(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	Version = "v1.2.3"
	if got := CurrentVersion(); got != "v1.2.3" {
		t.Errorf("CurrentVersion() = %q, want v1.2.3", got)
	}

	Version = ""
	if got := CurrentVersion(); got != "0.1.0" {
		t.Errorf("CurrentVersion() = %q, want 0.1.0", got)
	}
}
