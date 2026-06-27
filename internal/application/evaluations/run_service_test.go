package evaluations

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appproxy "proxygateway/internal/application/proxy"
	domainprofile "proxygateway/internal/domain/profile"
)

func TestRunServiceEvaluateFastestSelectsBestNodeAndNotifiesObserver(t *testing.T) {
	ports := newRunServiceFakePorts()
	ports.nodes = []appproxy.Node{
		{ID: "node_current", Name: "current", Enabled: true},
		{ID: "node_best", Name: "best", Enabled: true},
	}
	ports.currentNodeID = "node_current"
	ports.fetchNodeDuration = map[string]int64{
		"node_current": 200,
		"node_best":    50,
	}

	ok := NewRunService(ports.deps()).EvaluateFastest(context.Background(), Target{
		ID:      "profile_1",
		Type:    domainprofile.TypeFastest,
		TestURL: "https://example.test/generate_204",
	}, RuntimeSettings{GlobalConcurrency: 2})

	if !ok {
		t.Fatal("expected fastest evaluation to succeed")
	}
	if !reflect.DeepEqual(ports.releaseKeepNodeIDs, []string{"node_best"}) {
		t.Fatalf("released retained nodes with keep ids %v, want node_best", ports.releaseKeepNodeIDs)
	}
	if got := stringValue(ports.releaseUpdate.CurrentNodeID); got != "node_best" {
		t.Fatalf("selected node = %q, want node_best", got)
	}
	if got := stringValue(ports.releaseUpdate.SwitchReason); got != SwitchReasonCandidateClearlyBetter {
		t.Fatalf("switch reason = %q, want candidate_clearly_better", got)
	}
	if len(ports.fastestCandidateLogs) != 2 {
		t.Fatalf("fastest candidate logs = %d, want 2", len(ports.fastestCandidateLogs))
	}
	if len(ports.fastestSelectionReasons) != 1 || ports.fastestSelectionReasons[0] != SwitchReasonCandidateClearlyBetter {
		t.Fatalf("fastest selection logs = %v, want candidate_clearly_better", ports.fastestSelectionReasons)
	}
}

func TestRunServiceEvaluateFastestRecordsNoCandidateFailure(t *testing.T) {
	ports := newRunServiceFakePorts()

	ok := NewRunService(ports.deps()).EvaluateFastest(context.Background(), Target{
		ID:   "profile_1",
		Type: domainprofile.TypeFastest,
	}, RuntimeSettings{})

	if ok {
		t.Fatal("expected fastest evaluation with no candidates to fail")
	}
	if len(ports.stateUpdates) != 2 {
		t.Fatalf("state updates = %d, want running and final failure", len(ports.stateUpdates))
	}
	final := ports.stateUpdates[len(ports.stateUpdates)-1]
	if got := stringValue(final.State); got != ProfileStateNoCandidate {
		t.Fatalf("final state = %q, want no_candidate", got)
	}
	if got := stringValue(final.SwitchReason); got != SwitchReasonNoCandidate {
		t.Fatalf("switch reason = %q, want no_candidate", got)
	}
}

func TestRunServiceEvaluateFastestFrontSelectsBestChainLinkAndNotifiesObserver(t *testing.T) {
	ports := newRunServiceFakePorts()
	ports.nodes = []appproxy.Node{
		{ID: "front_current", Name: "current front", Enabled: true},
		{ID: "front_best", Name: "best front", Enabled: true},
	}
	ports.nodeByID["exit_1"] = appproxy.Node{ID: "exit_1", Name: "exit", Enabled: true}
	ports.currentFrontNodeID = "front_current"
	ports.currentExitNodeID = "exit_1"
	ports.chainLinkDuration = map[string]int64{
		"front_current->exit_1": 220,
		"front_best->exit_1":    40,
	}

	ok := NewRunService(ports.deps()).EvaluateFastestFront(context.Background(), Target{
		ID:          "profile_1",
		Type:        domainprofile.TypeChain,
		ExitNodeIDs: []string{"exit_1"},
	}, RuntimeSettings{GlobalConcurrency: 2})

	if !ok {
		t.Fatal("expected chain-link evaluation to succeed")
	}
	if !reflect.DeepEqual(ports.releaseKeepNodeIDs, []string{"front_best", "exit_1"}) {
		t.Fatalf("released retained nodes with keep ids %v, want front_best and exit_1", ports.releaseKeepNodeIDs)
	}
	if got := stringValue(ports.releaseUpdate.CurrentNodeID); got != "front_best" {
		t.Fatalf("selected front node = %q, want front_best", got)
	}
	if got := stringValue(ports.releaseUpdate.CurrentExitNodeID); got != "exit_1" {
		t.Fatalf("selected exit node = %q, want exit_1", got)
	}
	if len(ports.chainCandidateLogs) != 2 {
		t.Fatalf("chain candidate logs = %d, want 2", len(ports.chainCandidateLogs))
	}
	if len(ports.chainSelectionReasons) != 1 || ports.chainSelectionReasons[0] != SwitchReasonCandidateClearlyBetter {
		t.Fatalf("chain selection logs = %v, want candidate_clearly_better", ports.chainSelectionReasons)
	}
}

type runServiceFakePorts struct {
	nowMillis               int64
	nodes                   []appproxy.Node
	nodeByID                map[string]appproxy.Node
	currentNodeID           string
	currentFrontNodeID      string
	currentExitNodeID       string
	retained                map[string]bool
	fetchNodeDuration       map[string]int64
	fetchNodeErr            map[string]error
	chainLinkDuration       map[string]int64
	chainLinkErr            map[string]error
	fetchChainDuration      map[string]int64
	fetchChainErr           map[string]error
	stateUpdates            []StateUpdate
	releaseKeepNodeIDs      []string
	releaseUpdate           StateUpdate
	fastestCandidateLogs    []CandidateProbeResult[appproxy.Node]
	chainCandidateLogs      []ChainCandidateProbeLog
	fastestSelectionReasons []string
	chainSelectionReasons   []string
}

func newRunServiceFakePorts() *runServiceFakePorts {
	return &runServiceFakePorts{
		nowMillis:          1000,
		nodeByID:           map[string]appproxy.Node{},
		retained:           map[string]bool{},
		fetchNodeDuration:  map[string]int64{},
		chainLinkDuration:  map[string]int64{},
		fetchChainDuration: map[string]int64{},
	}
}

func (p *runServiceFakePorts) deps() RunServiceDeps {
	return RunServiceDeps{
		Clock:      p,
		Candidates: p,
		Paths:      p,
		State:      p,
		Probes:     p,
		Observer:   p,
	}
}

func (p *runServiceFakePorts) NowMillis() int64 {
	return p.nowMillis
}

func (p *runServiceFakePorts) CandidateNodes(context.Context, domainprofile.CandidateFilter) ([]appproxy.Node, error) {
	return append([]appproxy.Node(nil), p.nodes...), nil
}

func (p *runServiceFakePorts) LoadUsableNode(_ context.Context, nodeID string) (appproxy.Node, error) {
	if node, ok := p.nodeByID[nodeID]; ok {
		return node, nil
	}
	for _, node := range p.nodes {
		if node.ID == nodeID {
			return node, nil
		}
	}
	return appproxy.Node{}, errors.New("node not found")
}

func (p *runServiceFakePorts) ProfileRetainsNode(_ context.Context, _, nodeID string) bool {
	return p.retained[nodeID]
}

func (p *runServiceFakePorts) ProfileCurrentNodeID(context.Context, string) string {
	return p.currentNodeID
}

func (p *runServiceFakePorts) ProfileCurrentChainPath(context.Context, string) (string, string) {
	return p.currentFrontNodeID, p.currentExitNodeID
}

func (p *runServiceFakePorts) SelectedProfileNodeStillValid(_ context.Context, _ Target, nodeID string) bool {
	return nodeID != ""
}

func (p *runServiceFakePorts) SelectedChainPathStillValid(_ context.Context, _ Target, frontNodeID, exitNodeID string) bool {
	return frontNodeID != "" && exitNodeID != ""
}

func (p *runServiceFakePorts) RetainedCurrentChainPathExists(context.Context, Target) bool {
	return p.currentFrontNodeID != "" && p.currentExitNodeID != ""
}

func (p *runServiceFakePorts) RetainedChainLinkExitPathExists(_ context.Context, _ Target, exitNodeID string) bool {
	return p.currentFrontNodeID != "" && p.currentExitNodeID == exitNodeID
}

func (p *runServiceFakePorts) UpdateState(_ context.Context, _ Target, update StateUpdate) bool {
	p.stateUpdates = append(p.stateUpdates, update)
	return true
}

func (p *runServiceFakePorts) UpdateStateAndReleaseRetained(_ context.Context, _ Target, keepNodeIDs []string, update StateUpdate) bool {
	p.releaseKeepNodeIDs = append([]string(nil), keepNodeIDs...)
	p.releaseUpdate = update
	return true
}

func (p *runServiceFakePorts) FetchNode(_ context.Context, node appproxy.Node, _ string) (int64, int, error) {
	if err := p.fetchNodeErr[node.ID]; err != nil {
		return 0, 0, err
	}
	return p.fetchNodeDuration[node.ID], 204, nil
}

func (p *runServiceFakePorts) ProbeChainLink(_ context.Context, frontNode, exitNode appproxy.Node) (int64, error) {
	key := frontNode.ID + "->" + exitNode.ID
	if err := p.chainLinkErr[key]; err != nil {
		return 0, err
	}
	return p.chainLinkDuration[key], nil
}

func (p *runServiceFakePorts) FetchChain(_ context.Context, frontNode, exitNode appproxy.Node, _ string) (int64, int, error) {
	key := frontNode.ID + "->" + exitNode.ID
	if err := p.fetchChainErr[key]; err != nil {
		return 0, 0, err
	}
	return p.fetchChainDuration[key], 204, nil
}

func (p *runServiceFakePorts) LogFastestCandidate(_ context.Context, _ string, result CandidateProbeResult[appproxy.Node]) {
	p.fastestCandidateLogs = append(p.fastestCandidateLogs, result)
}

func (p *runServiceFakePorts) LogChainCandidate(_ context.Context, _ string, result ChainCandidateProbeLog) {
	p.chainCandidateLogs = append(p.chainCandidateLogs, result)
}

func (p *runServiceFakePorts) LogFastestSelection(_ context.Context, _ Target, _ int, _ FastestProbeSummary, _, _, switchReason string) {
	p.fastestSelectionReasons = append(p.fastestSelectionReasons, switchReason)
}

func (p *runServiceFakePorts) LogChainSelection(_ context.Context, _ Target, _ int, _ ChainProbeSummary, _, _, _, _, switchReason string) {
	p.chainSelectionReasons = append(p.chainSelectionReasons, switchReason)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
