package evaluations

type ConfigVersionGuardInput struct {
	RequestedConfigVersion int64
	CurrentConfigVersion   int64
}

type ConfigVersionGuard struct {
	Superseded           bool
	ReasonCode           string
	CurrentConfigVersion int64
}

func CheckConfigVersion(input ConfigVersionGuardInput) ConfigVersionGuard {
	if input.RequestedConfigVersion <= 0 || input.CurrentConfigVersion <= 0 {
		return ConfigVersionGuard{}
	}
	if input.RequestedConfigVersion == input.CurrentConfigVersion {
		return ConfigVersionGuard{}
	}
	return ConfigVersionGuard{
		Superseded:           true,
		ReasonCode:           "superseded_by_config_version",
		CurrentConfigVersion: input.CurrentConfigVersion,
	}
}
