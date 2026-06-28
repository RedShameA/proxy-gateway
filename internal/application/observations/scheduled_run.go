package observations

import appmaintenance "proxygateway/internal/application/maintenance"

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
		TriggerSource: appmaintenance.TriggerScheduled,
		Scope:         appmaintenance.NodeObservationScopeAllNodes,
		ProbeURL:      probeURL,
		Targets:       append([]NodeTarget{}, targets...),
		NotifyRunner:  !hasUnfinishedAggregate,
	}
	if hasUnfinishedAggregate {
		plan.FinishImmediately = true
		plan.Result = appmaintenance.ResultSkipped
		plan.ReasonCode = appmaintenance.ReasonPreviousRunStillRunning
	}
	return plan
}

func PlanSubscriptionRefreshAggregateRun(targets []NodeTarget, probeURL string) ScheduledAggregateRunPlan {
	return PlanSubscriptionObservationAggregateRun(targets, probeURL, appmaintenance.TriggerSubscriptionRefresh)
}

func PlanSubscriptionObservationAggregateRun(targets []NodeTarget, probeURL, triggerSource string) ScheduledAggregateRunPlan {
	if len(targets) == 0 {
		return ScheduledAggregateRunPlan{}
	}
	return ScheduledAggregateRunPlan{
		CreateRun:     true,
		TriggerSource: triggerSource,
		Scope:         appmaintenance.NodeObservationScopeAllNodes,
		ProbeURL:      probeURL,
		Targets:       append([]NodeTarget{}, targets...),
		NotifyRunner:  true,
	}
}
