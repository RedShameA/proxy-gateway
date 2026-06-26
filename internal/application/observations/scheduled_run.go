package observations

type ScheduledAggregateRunPlan struct {
	CreateRun         bool
	FinishImmediately bool
	TriggerSource     string
	Scope             string
	ProbeURL          string
	Targets           []NodeTarget
	Result            string
	ReasonCode        string
	NotifyRunner      bool
}

func PlanScheduledAggregateRun(targets []NodeTarget, probeURL string, hasUnfinishedAggregate bool) ScheduledAggregateRunPlan {
	if len(targets) == 0 {
		return ScheduledAggregateRunPlan{}
	}
	plan := ScheduledAggregateRunPlan{
		CreateRun:     true,
		TriggerSource: "scheduled",
		Scope:         "all_nodes",
		ProbeURL:      probeURL,
		Targets:       append([]NodeTarget{}, targets...),
		NotifyRunner:  !hasUnfinishedAggregate,
	}
	if hasUnfinishedAggregate {
		plan.FinishImmediately = true
		plan.Result = "skipped"
		plan.ReasonCode = "previous_run_still_running"
	}
	return plan
}

func PlanSubscriptionRefreshAggregateRun(targets []NodeTarget, probeURL string) ScheduledAggregateRunPlan {
	if len(targets) == 0 {
		return ScheduledAggregateRunPlan{}
	}
	return ScheduledAggregateRunPlan{
		CreateRun:     true,
		TriggerSource: "subscription_refresh",
		Scope:         "all_nodes",
		ProbeURL:      probeURL,
		Targets:       append([]NodeTarget{}, targets...),
		NotifyRunner:  true,
	}
}
