package settings

import "errors"

const (
	DefaultConnectTimeoutSeconds = 10
	DefaultProbeTimeoutSeconds   = 10
	EvaluationSettingsRangeError = "评估设置数值必须满足正数或非负数要求"
)

var ErrEvaluationSettingsRange = errors.New(EvaluationSettingsRangeError)

func NormalizeEvaluation(settings EvaluationSettings) EvaluationSettings {
	if settings.GlobalConcurrency <= 0 {
		settings.GlobalConcurrency = 1
	}
	if settings.ConnectTimeoutSeconds <= 0 {
		settings.ConnectTimeoutSeconds = DefaultConnectTimeoutSeconds
	}
	if settings.ProbeTimeoutSeconds <= 0 {
		settings.ProbeTimeoutSeconds = DefaultProbeTimeoutSeconds
	}
	return settings
}

func ValidateEvaluation(settings EvaluationSettings) error {
	if settings.GlobalConcurrency <= 0 ||
		settings.DefaultMinEvaluationIntervalSeconds < 0 ||
		settings.SingleCandidateLimit < 0 ||
		settings.ChainCandidateLimit < 0 ||
		settings.ConnectTimeoutSeconds <= 0 ||
		settings.ProbeTimeoutSeconds <= 0 {
		return ErrEvaluationSettingsRange
	}
	return nil
}
