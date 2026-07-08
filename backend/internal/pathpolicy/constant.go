package pathpolicy

const (
	PathClassWorkspace       PathClass = "workspace"
	PathClassSecret          PathClass = "secret"
	PathClassSystem          PathClass = "system"
	PathClassTemp            PathClass = "temp"
	PathClassBuildCache      PathClass = "build-cache"
	PathClassCredentialStore PathClass = "credential-store"
	PathClassUnknown         PathClass = "unknown"
)