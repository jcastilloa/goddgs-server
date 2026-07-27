package buildinfo

const defaultVersion = "0.1.0"

var Version = defaultVersion

func CurrentVersion() string {
	if Version == "" {
		return defaultVersion
	}
	return Version
}
