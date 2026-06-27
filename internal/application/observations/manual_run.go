package observations

import (
	"errors"
	"strings"
)

var ErrObservationTargetNotFound = errors.New("observation target not found")

type NodeTarget struct {
	ID   string
	Name string
}

type ManualRunRepository interface {
	EnabledNodeByID(nodeID string) (NodeTarget, bool, error)
	AllEnabledNodes() ([]NodeTarget, error)
}

type ManualRunCommand struct {
	NodeID          string
	NodeIDs         []string
	ProbeURL        string
	LegacyTestURL   string
	DefaultProbeURL string
}

type ManualRunPlan struct {
	Scope                         string
	ProbeURL                      string
	Targets                       []NodeTarget
	CancelUnfinishedAggregateRuns bool
}

func PlanManualRun(repo ManualRunRepository, command ManualRunCommand) (ManualRunPlan, error) {
	nodeID := strings.TrimSpace(command.NodeID)
	if nodeID != "" {
		target, ok, err := repo.EnabledNodeByID(nodeID)
		if err != nil {
			return ManualRunPlan{}, err
		}
		if !ok {
			return ManualRunPlan{}, ErrObservationTargetNotFound
		}
		return ManualRunPlan{
			Scope:    ScopeSingleNode,
			ProbeURL: effectiveProbeURL(command),
			Targets:  []NodeTarget{target},
		}, nil
	}

	normalizedIDs := normalizeStringList(command.NodeIDs)
	if len(normalizedIDs) == 1 {
		target, ok, err := repo.EnabledNodeByID(normalizedIDs[0])
		if err != nil {
			return ManualRunPlan{}, err
		}
		if !ok {
			return ManualRunPlan{}, ErrObservationTargetNotFound
		}
		return ManualRunPlan{
			Scope:    ScopeSingleNode,
			ProbeURL: effectiveProbeURL(command),
			Targets:  []NodeTarget{target},
		}, nil
	}

	targets := make([]NodeTarget, 0, len(normalizedIDs))
	if len(normalizedIDs) > 1 {
		for _, id := range normalizedIDs {
			target, ok, err := repo.EnabledNodeByID(id)
			if err != nil {
				return ManualRunPlan{}, err
			}
			if ok {
				targets = append(targets, target)
			}
		}
	} else {
		var err error
		targets, err = repo.AllEnabledNodes()
		if err != nil {
			return ManualRunPlan{}, err
		}
	}

	return ManualRunPlan{
		Scope:                         ScopeAllNodes,
		ProbeURL:                      effectiveProbeURL(command),
		Targets:                       targets,
		CancelUnfinishedAggregateRuns: true,
	}, nil
}

func effectiveProbeURL(command ManualRunCommand) string {
	return EffectiveProbeURL(command.ProbeURL, command.LegacyTestURL, command.DefaultProbeURL)
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
