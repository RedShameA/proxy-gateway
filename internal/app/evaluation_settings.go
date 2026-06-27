package app

import (
	"context"
	"time"

	appsettings "proxygateway/internal/application/settings"
)

func (g *Gateway) loadEvaluationSettings() (evaluationSettings, error) {
	settings, err := g.systemSettingsRepo.LoadEvaluation(context.Background())
	return normalizeEvaluationSettings(evaluationSettingsFromApplication(settings)), err
}

func normalizeEvaluationSettings(settings evaluationSettings) evaluationSettings {
	return evaluationSettingsFromApplication(appsettings.NormalizeEvaluation(applicationEvaluationSettings(settings)))
}

func validateEvaluationSettings(settings evaluationSettings) error {
	return appsettings.ValidateEvaluation(applicationEvaluationSettings(settings))
}

func (g *Gateway) saveEvaluationSettings(settings evaluationSettings) error {
	return g.systemSettingsRepo.SaveEvaluation(context.Background(), applicationEvaluationSettings(settings))
}

func evaluationSettingsFromApplication(settings appsettings.EvaluationSettings) evaluationSettings {
	return evaluationSettings{
		GlobalConcurrency:                   settings.GlobalConcurrency,
		DefaultMinEvaluationIntervalSeconds: settings.DefaultMinEvaluationIntervalSeconds,
		SingleCandidateLimit:                settings.SingleCandidateLimit,
		ChainCandidateLimit:                 settings.ChainCandidateLimit,
		ConnectTimeoutSeconds:               settings.ConnectTimeoutSeconds,
		ProbeTimeoutSeconds:                 settings.ProbeTimeoutSeconds,
	}
}

func applicationEvaluationSettings(settings evaluationSettings) appsettings.EvaluationSettings {
	return appsettings.EvaluationSettings{
		GlobalConcurrency:                   settings.GlobalConcurrency,
		DefaultMinEvaluationIntervalSeconds: settings.DefaultMinEvaluationIntervalSeconds,
		SingleCandidateLimit:                settings.SingleCandidateLimit,
		ChainCandidateLimit:                 settings.ChainCandidateLimit,
		ConnectTimeoutSeconds:               settings.ConnectTimeoutSeconds,
		ProbeTimeoutSeconds:                 settings.ProbeTimeoutSeconds,
	}
}

func (settings evaluationSettings) probeDialTimeouts() dialTimeouts {
	settings = normalizeEvaluationSettings(settings)
	return dialTimeouts{
		ConnectTimeout: time.Duration(settings.ConnectTimeoutSeconds) * time.Second,
		Deadline:       time.Now().Add(time.Duration(settings.ProbeTimeoutSeconds) * time.Second),
	}
}
