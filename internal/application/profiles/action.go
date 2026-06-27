package profiles

import (
	"errors"

	appmaintenance "proxygateway/internal/application/maintenance"
)

var (
	ErrProfileTypeNotEvaluable = errors.New(appmaintenance.ReasonProfileTypeNotEvaluable)
	ErrNoCurrentPathToSwitch   = errors.New("no current path to switch from")
	ErrUnknownAction           = errors.New("unknown action")
)

type ActionPlan struct {
	CreateSwitchRun   bool
	EnqueueEvaluation bool
	ResponseState     string
	SwitchReason      string
	SwitchRunDetail   map[string]any
}

func BuildActionPlan(cfg ConfigRecord, action string) (ActionPlan, error) {
	switch action {
	case ActionEvaluate:
		if !TypeNeedsEvaluation(cfg.Type) {
			return ActionPlan{}, ErrProfileTypeNotEvaluable
		}
		return ActionPlan{
			EnqueueEvaluation: true,
			ResponseState:     appmaintenance.StateQueued,
		}, nil
	case ActionSwitchToBestObserved:
		if cfg.CurrentNodeID == "" {
			return ActionPlan{}, ErrNoCurrentPathToSwitch
		}
		return ActionPlan{
			CreateSwitchRun:   true,
			EnqueueEvaluation: true,
			ResponseState:     appmaintenance.StateFinished,
			SwitchReason:      ManualSwitchReason,
			SwitchRunDetail: map[string]any{
				"profile_id":     cfg.ID,
				"config_version": cfg.ConfigVersion,
			},
		}, nil
	default:
		return ActionPlan{}, ErrUnknownAction
	}
}
