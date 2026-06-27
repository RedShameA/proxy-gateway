package observations

type MaintenanceRunContext struct {
	ID            string
	TriggerSource string
	Detail        map[string]any
}

type MaintenanceRunExecution struct {
	Outcome RunOutcome
	Detail  map[string]any
	Results []RunResult
}

func ExecuteMaintenanceRun(repo PersistenceRepository, lookup CountryLookup, run MaintenanceRunContext, targets []ExecutableTarget, concurrency int, now ObservationClock) MaintenanceRunExecution {
	detail := copyDetail(run.Detail)
	if len(targets) == 0 {
		outcome := BuildNoTargetOutcome()
		applyOutcomeDetail(detail, outcome)
		return MaintenanceRunExecution{Outcome: outcome, Detail: detail}
	}
	results := ExecuteBatch(repo, lookup, targets, concurrency, now)
	outcome := BuildCompletedOutcome(run.TriggerSource, results)
	applyOutcomeDetail(detail, outcome)
	return MaintenanceRunExecution{
		Outcome: outcome,
		Detail:  detail,
		Results: results,
	}
}

func applyOutcomeDetail(detail map[string]any, outcome RunOutcome) {
	detail["success_count"] = outcome.SuccessCount
	detail["failure_count"] = outcome.FailureCount
	detail["failure_reasons"] = outcome.FailureReasons
	detail["sample_failures"] = outcome.SampleFailures
}

func copyDetail(detail map[string]any) map[string]any {
	if detail == nil {
		return map[string]any{}
	}
	copied := make(map[string]any, len(detail))
	for key, value := range detail {
		copied[key] = value
	}
	return copied
}
