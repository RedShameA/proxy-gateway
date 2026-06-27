package profiles

import domainprofile "proxygateway/internal/domain/profile"

type ConfigCreatePlan struct {
	Config                           ConfigRecord
	EnqueueEvaluation                bool
	EnqueueUnknownCountryObservation bool
}

type ConfigUpdatePlan struct {
	Config                           ConfigRecord
	EvaluationChanged                bool
	ResetCurrentPath                 bool
	ReleaseRetainedNodes             bool
	EnqueueEvaluation                bool
	EnqueueUnknownCountryObservation bool
}

func BuildCreateConfigPlan(id string, req PatchRequest, deps ConfigValidationDeps) (ConfigCreatePlan, error) {
	cfg := DefaultConfig(id)
	ApplyConfigPatch(&cfg, req)
	if err := ValidateConfig(&cfg, deps); err != nil {
		return ConfigCreatePlan{}, err
	}
	return ConfigCreatePlan{
		Config:                           cfg,
		EnqueueEvaluation:                cfg.AutoEvaluationEnabled && TypeNeedsEvaluation(cfg.Type),
		EnqueueUnknownCountryObservation: len(cfg.EgressCountries) > 0,
	}, nil
}

func BuildUpdateConfigPlan(original ConfigRecord, req PatchRequest, deps ConfigValidationDeps) (ConfigUpdatePlan, error) {
	updated := original
	ApplyConfigPatch(&updated, req)
	if err := ValidateConfig(&updated, deps); err != nil {
		return ConfigUpdatePlan{}, err
	}
	plan := domainprofile.PlanConfigUpdate(original.DomainSnapshot(), updated.DomainSnapshot())
	updated.ApplyDomainSnapshot(plan.Config)
	return ConfigUpdatePlan{
		Config:                           updated,
		EvaluationChanged:                plan.EvaluationChanged,
		ResetCurrentPath:                 plan.ResetCurrentPath,
		ReleaseRetainedNodes:             plan.ReleaseRetainedNodes,
		EnqueueEvaluation:                plan.EnqueueEvaluation,
		EnqueueUnknownCountryObservation: plan.EnqueueUnknownCountryObservation,
	}, nil
}

func TypeNeedsEvaluation(profileType string) bool {
	return domainprofile.TypeNeedsEvaluation(profileType)
}
