package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apperrors "proxygateway/internal/application/apperrors"
	applicationnodes "proxygateway/internal/application/nodes"
	appobservations "proxygateway/internal/application/observations"
	probinginfra "proxygateway/internal/infrastructure/probing"
)

const defaultEgressIPProbeURL = "https://cloudflare.com/cdn-cgi/trace"

type manualNodeObservationRequest struct {
	TestURL  string
	ProbeURL string
	NodeID   string
	NodeIDs  []string
}

type manualNodeObservationResult struct {
	ObservedNodes int
	RunID         string
}

func (g *Gateway) runManualNodeObservations(req manualNodeObservationRequest) (manualNodeObservationResult, error) {
	plan, err := appobservations.PlanManualRun(manualObservationRunRepository{g: g}, appobservations.ManualRunCommand{
		NodeID:          req.NodeID,
		NodeIDs:         req.NodeIDs,
		ProbeURL:        req.ProbeURL,
		LegacyTestURL:   req.TestURL,
		DefaultProbeURL: defaultEgressIPProbeURL,
	})
	if err != nil {
		if errors.Is(err, appobservations.ErrObservationTargetNotFound) {
			return manualNodeObservationResult{}, apperrors.New(apperrors.KindNotFound, err.Error(), err)
		}
		return manualNodeObservationResult{}, apperrors.New(apperrors.KindInternal, err.Error(), err)
	}
	if plan.CancelUnfinishedAggregateRuns {
		if err := g.cancelUnfinishedNodeObservationAggregateRuns("replaced_by_manual_run"); err != nil {
			return manualNodeObservationResult{}, apperrors.New(apperrors.KindInternal, "cancel previous node observation runs", err)
		}
	}
	run, err := g.createNodeObservationRun("manual", plan.Scope, toObservationNodeRecords(plan.Targets), plan.ProbeURL)
	if err != nil {
		return manualNodeObservationResult{}, apperrors.New(apperrors.KindInternal, "create maintenance run", err)
	}
	if err := g.runNodeObservationMaintenanceRun(run.ID); err != nil {
		return manualNodeObservationResult{}, apperrors.New(apperrors.KindInternal, "run node observation", err)
	}
	finished, _ := g.loadMaintenanceRun(run.ID)
	successCount, _ := maintenanceRunDetail(finished)["success_count"].(float64)
	return manualNodeObservationResult{ObservedNodes: int(successCount), RunID: run.ID}, nil
}

func (g *Gateway) runNodeObservationMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := maintenanceRunDetail(run)
	probeURL, _ := detail["probe_url"].(string)
	probeURL = appobservations.EffectiveProbeURL(probeURL, defaultEgressIPProbeURL)
	targets := g.nodeObservationRunTargets(run, detail)
	if len(targets) == 0 {
		execution := appobservations.ExecuteMaintenanceRun(nil, nil, appobservations.MaintenanceRunContext{
			ID:            run.ID,
			TriggerSource: run.TriggerSource,
			Detail:        detail,
		}, nil, 0, unixMillisNow)
		outcome := execution.Outcome
		return g.finishMaintenanceRun(run.ID, outcome.Result, outcome.ReasonCode, outcome.FinishedCount, execution.Detail, outcome.LastError)
	}
	settings, err := g.loadMaintenanceSettings()
	if err != nil {
		return err
	}
	evalSettings, err := g.loadEvaluationSettings()
	if err != nil {
		return err
	}
	concurrency := settings.NodeObservationConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	lookup := geoIPCountryLookup{geoIP: g.geoIP}
	batchTargets := make([]appobservations.ExecutableTarget, 0, len(targets))
	for _, node := range targets {
		batchTargets = append(batchTargets, appobservations.ExecutableTarget{
			Target:   appobservations.NodeTarget{ID: node.ID, Name: node.Name},
			Executor: nodeObservationProbeExecutor{client: g.probeClient(), node: node, probeURL: probeURL, settings: evalSettings},
		})
	}
	execution := appobservations.ExecuteMaintenanceRun(g.nodeObservationRepo, lookup, appobservations.MaintenanceRunContext{
		ID:            run.ID,
		TriggerSource: run.TriggerSource,
		Detail:        detail,
	}, batchTargets, concurrency, unixMillisNow)
	for _, result := range execution.Results {
		g.logNodeObservationResult(run.ID, result)
	}
	outcome := execution.Outcome
	if err := g.finishMaintenanceRun(run.ID, outcome.Result, outcome.ReasonCode, outcome.FinishedCount, execution.Detail, outcome.LastError); err != nil {
		return err
	}
	if outcome.EnqueueWaitingProfiles {
		g.enqueueProfileEvaluationsWaitingForObservation()
	}
	return nil
}

type manualObservationRunRepository struct {
	g *Gateway
}

func (r manualObservationRunRepository) EnabledNodeByID(nodeID string) (appobservations.NodeTarget, bool, error) {
	node, err := r.g.loadNode(nodeID)
	if err != nil {
		if errors.Is(err, applicationnodes.ErrNodeNotFound) {
			return appobservations.NodeTarget{}, false, nil
		}
		return appobservations.NodeTarget{}, false, err
	}
	if !node.Enabled {
		return appobservations.NodeTarget{}, false, nil
	}
	return appobservations.NodeTarget{ID: node.ID, Name: node.Name}, true, nil
}

func (r manualObservationRunRepository) AllEnabledNodes() ([]appobservations.NodeTarget, error) {
	nodes, err := r.g.nodeRepo.ListEnabledObservationTargets(context.Background())
	if err != nil {
		return nil, err
	}
	targets := make([]appobservations.NodeTarget, 0, len(nodes))
	for _, node := range nodes {
		targets = append(targets, appobservations.NodeTarget{ID: node.ID, Name: node.Name})
	}
	return targets, nil
}

func toObservationNodeRecords(targets []appobservations.NodeTarget) []nodeRecord {
	out := make([]nodeRecord, 0, len(targets))
	for _, target := range targets {
		out = append(out, nodeRecord{ID: target.ID, Name: target.Name, Enabled: true})
	}
	return out
}

func toObservationNodeTargets(targets []nodeRecord) []appobservations.NodeTarget {
	out := make([]appobservations.NodeTarget, 0, len(targets))
	for _, target := range targets {
		out = append(out, appobservations.NodeTarget{ID: target.ID, Name: target.Name})
	}
	return out
}

func (g *Gateway) createNodeObservationRun(triggerSource, scope string, targets []nodeRecord, probeURL string) (maintenanceRunRecord, error) {
	plan := appobservations.BuildRunCreatePlan(scope, toObservationNodeTargets(targets), probeURL)
	return g.createMaintenanceRun(maintenanceTaskNodeObservation, triggerSource, plan.TargetID, plan.TargetLabel, plan.TotalCount, plan.Detail)
}

func (g *Gateway) nodeObservationRunTargets(run maintenanceRunRecord, detail map[string]any) []nodeRecord {
	ids := appobservations.NodeIDsFromRunDetail(run.TargetID, detail)
	targets := make([]nodeRecord, 0, len(ids))
	for _, id := range ids {
		node, err := g.loadNode(id)
		if err == nil && node.Enabled {
			targets = append(targets, node)
		}
	}
	return targets
}

func (g *Gateway) observeNode(node nodeRecord, probeURL string, settings evaluationSettings) (bool, error) {
	result := appobservations.ExecuteNodeObservation(
		g.nodeObservationRepo,
		geoIPCountryLookup{geoIP: g.geoIP},
		nodeObservationProbeExecutor{client: g.probeClient(), node: node, probeURL: probeURL, settings: settings},
		appobservations.NodeTarget{ID: node.ID, Name: node.Name},
		unixMillisNow(),
	)
	if result.OK {
		return true, nil
	}
	if strings.TrimSpace(result.Error) == "" {
		return false, nil
	}
	return false, errors.New(result.Error)
}

type nodeObservationProbeExecutor struct {
	client   probinginfra.Client
	node     nodeRecord
	probeURL string
	settings evaluationSettings
}

func (e nodeObservationProbeExecutor) Probe() (appobservations.ProbePayload, error) {
	result, err := e.client.FetchThroughNode(e.node, e.probeURL, e.settings.probeDialTimeouts())
	if err != nil {
		return appobservations.ProbePayload{}, err
	}
	if result.HTTPStatus < 200 || result.HTTPStatus >= 400 {
		err := fmt.Errorf("egress probe returned %d %s", result.HTTPStatus, http.StatusText(result.HTTPStatus))
		return appobservations.ProbePayload{}, err
	}
	return appobservations.ProbePayload{
		Raw:       result.Body,
		LatencyMS: result.DurationMS,
	}, nil
}

type geoIPCountryLookup struct {
	geoIP geoIPCountryService
}

func (l geoIPCountryLookup) LookupCountry(ip string) string {
	if l.geoIP == nil {
		return ""
	}
	return l.geoIP.LookupCountry(ip)
}

func (g *Gateway) nodeObservation(nodeID string) map[string]any {
	return g.nodeObservationSnapshot(nodeID).Map()
}

func (g *Gateway) nodeObservationSnapshot(nodeID string) applicationnodes.ObservationSnapshot {
	observation, found, err := g.nodeRepo.LoadObservation(context.Background(), nodeID)
	if err != nil {
		return applicationnodes.UnavailableObservationSnapshot()
	}
	return applicationnodes.ObservationSnapshotFromRecord(observation, found)
}
