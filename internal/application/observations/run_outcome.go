package observations

import (
	"strings"

	appmaintenance "proxygateway/internal/application/maintenance"
)

type RunResult struct {
	NodeID string
	Name   string
	OK     bool
	Error  string
}

type RunOutcome struct {
	Result                 string
	ReasonCode             string
	FinishedCount          int
	SuccessCount           int
	FailureCount           int
	LastError              string
	FailureReasons         map[string]int
	SampleFailures         []map[string]any
	EnqueueWaitingProfiles bool
}

func BuildNoTargetOutcome() RunOutcome {
	return RunOutcome{
		Result:         appmaintenance.ResultSkipped,
		ReasonCode:     appmaintenance.ReasonNoTargets,
		FailureReasons: map[string]int{},
		SampleFailures: []map[string]any{},
	}
}

func BuildCompletedOutcome(triggerSource string, results []RunResult) RunOutcome {
	outcome := RunOutcome{
		Result:         appmaintenance.ResultSuccess,
		ReasonCode:     appmaintenance.ReasonCompleted,
		FinishedCount:  len(results),
		FailureReasons: map[string]int{},
		SampleFailures: []map[string]any{},
	}
	for _, result := range results {
		if result.OK {
			outcome.SuccessCount++
			continue
		}
		outcome.LastError = result.Error
		reason := strings.TrimSpace(result.Error)
		if reason == "" {
			reason = "observation_failed"
		}
		outcome.FailureReasons[reason]++
		if len(outcome.SampleFailures) < 10 {
			outcome.SampleFailures = append(outcome.SampleFailures, map[string]any{
				"node_id": result.NodeID,
				"name":    result.Name,
				"error":   result.Error,
			})
		}
	}
	outcome.FailureCount = len(results) - outcome.SuccessCount
	if outcome.FailureCount > 0 && outcome.SuccessCount > 0 {
		outcome.ReasonCode = appmaintenance.ReasonPartialFailure
	} else if outcome.FailureCount > 0 {
		outcome.Result = appmaintenance.ResultFailure
		outcome.ReasonCode = appmaintenance.ReasonAllFailed
	}
	outcome.EnqueueWaitingProfiles = isSubscriptionObservationTrigger(triggerSource)
	return outcome
}

func isSubscriptionObservationTrigger(triggerSource string) bool {
	return triggerSource == appmaintenance.TriggerSubscriptionRefresh ||
		triggerSource == appmaintenance.TriggerSubscriptionImport
}
