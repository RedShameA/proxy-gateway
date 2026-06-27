package evaluations

import appmaintenance "proxygateway/internal/application/maintenance"

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
		ReasonCode:           appmaintenance.ReasonSupersededByConfigVersion,
		CurrentConfigVersion: input.CurrentConfigVersion,
	}
}
