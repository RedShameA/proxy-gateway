package proxy

const (
	FailureStageAuthentication   = "authentication"
	FailureStageProfileSelection = "profile_selection"
	FailureStagePathSelection    = "path_selection"
	FailureStageDial             = "dial"
	FailureStageProxyHandshake   = "proxy_handshake"
	FailureStageUpstream         = "upstream"
)

type Failure struct {
	Stage string
	Error string
}

func MissingProxyAuthenticationFailure() Failure {
	return Failure{Stage: FailureStageAuthentication, Error: "proxy authentication required"}
}

func InvalidProxyAuthenticationFailure() Failure {
	return Failure{Stage: FailureStageAuthentication, Error: "invalid proxy authentication"}
}

func InvalidProxyCredentialsFailure() Failure {
	return Failure{Stage: FailureStageAuthentication, Error: "invalid proxy credentials"}
}

func AccessProfileNotFoundFailure() Failure {
	return Failure{Stage: FailureStageProfileSelection, Error: "access profile not found"}
}

func ClassifyProxyPathFailure(errorText string) Failure {
	if errorText == "access profile not found" {
		return AccessProfileNotFoundFailure()
	}
	return Failure{Stage: FailureStagePathSelection, Error: errorText}
}
