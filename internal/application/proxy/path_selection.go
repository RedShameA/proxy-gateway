package proxy

import (
	"errors"

	appprofiles "proxygateway/internal/application/profiles"
	domainprofile "proxygateway/internal/domain/profile"
)

var ErrNoUsableProxyPath = errors.New("access profile has no usable proxy path")

type SelectedPath struct {
	Credential        CredentialRecord
	ProfileID         string
	Profile           string
	ProfileIdentifier string
	Node              Node
	FrontNode         Node
	ExitNode          Node
}

type PathSelectionDeps struct {
	CandidateNodes                    func(domainprofile.CandidateFilter) ([]Node, error)
	UsableNodes                       func([]Node) []Node
	RandomIndex                       func(int) (int, error)
	ChainPathMatchesProfile           func(appprofiles.ConfigRecord, string, string) bool
	LoadUsableNode                    func(string) (Node, error)
	ProfileNodeMatchesCandidateFilter func(profileID, nodeID string, filter domainprofile.CandidateFilter) bool
}

func SelectPathForCredential(credential CredentialRecord, cfg appprofiles.ConfigRecord, deps PathSelectionDeps) (SelectedPath, error) {
	switch cfg.Type {
	case "random":
		return selectRandomPath(credential, cfg, deps)
	case "chain":
		return selectChainPath(credential, cfg, deps)
	default:
		return selectSinglePath(credential, cfg, deps)
	}
}

func selectRandomPath(credential CredentialRecord, cfg appprofiles.ConfigRecord, deps PathSelectionDeps) (SelectedPath, error) {
	nodes, err := candidateNodes(cfg.CandidateFilter(), deps.CandidateNodes)
	if err != nil || len(nodes) == 0 {
		return SelectedPath{}, ErrNoUsableProxyPath
	}
	nodes = usableNodes(nodes, deps.UsableNodes)
	if len(nodes) == 0 {
		return SelectedPath{}, ErrNoUsableProxyPath
	}
	idx, err := randomIndex(len(nodes), deps.RandomIndex)
	if err != nil {
		return SelectedPath{}, err
	}
	return singleSelectedPath(credential, cfg, nodes[idx]), nil
}

func selectChainPath(credential CredentialRecord, cfg appprofiles.ConfigRecord, deps PathSelectionDeps) (SelectedPath, error) {
	exitNodeID := cfg.CurrentExitNodeID
	if exitNodeID == "" && len(cfg.ExitNodeIDs) == 1 {
		exitNodeID = cfg.ExitNodeIDs[0]
	}
	if !appprofiles.StateHasReusablePath(cfg.State) || cfg.CurrentNodeID == "" || exitNodeID == "" {
		return SelectedPath{}, ErrNoUsableProxyPath
	}
	if deps.ChainPathMatchesProfile != nil && !deps.ChainPathMatchesProfile(cfg, cfg.CurrentNodeID, exitNodeID) {
		return SelectedPath{}, ErrNoUsableProxyPath
	}
	frontNode, err := loadUsableNode(cfg.CurrentNodeID, deps.LoadUsableNode)
	if err != nil {
		return SelectedPath{}, err
	}
	exitNode, err := loadUsableNode(exitNodeID, deps.LoadUsableNode)
	if err != nil {
		return SelectedPath{}, err
	}
	return SelectedPath{
		Credential:        credential,
		ProfileID:         credential.ProfileID,
		Profile:           cfg.Name,
		ProfileIdentifier: cfg.EffectiveProfileIdentifier(),
		FrontNode:         frontNode,
		ExitNode:          exitNode,
	}, nil
}

func selectSinglePath(credential CredentialRecord, cfg appprofiles.ConfigRecord, deps PathSelectionDeps) (SelectedPath, error) {
	if !appprofiles.StateHasReusablePath(cfg.State) || cfg.CurrentNodeID == "" {
		return SelectedPath{}, ErrNoUsableProxyPath
	}
	if cfg.Type == "fastest" && deps.ProfileNodeMatchesCandidateFilter != nil && !deps.ProfileNodeMatchesCandidateFilter(cfg.ID, cfg.CurrentNodeID, cfg.CandidateFilter()) {
		return SelectedPath{}, ErrNoUsableProxyPath
	}
	node, err := loadUsableNode(cfg.CurrentNodeID, deps.LoadUsableNode)
	if err != nil {
		return SelectedPath{}, err
	}
	return singleSelectedPath(credential, cfg, node), nil
}

func singleSelectedPath(credential CredentialRecord, cfg appprofiles.ConfigRecord, node Node) SelectedPath {
	return SelectedPath{
		Credential:        credential,
		ProfileID:         credential.ProfileID,
		Profile:           cfg.Name,
		ProfileIdentifier: cfg.EffectiveProfileIdentifier(),
		Node:              node,
	}
}

func candidateNodes(filter domainprofile.CandidateFilter, load func(domainprofile.CandidateFilter) ([]Node, error)) ([]Node, error) {
	if load == nil {
		return nil, ErrNoUsableProxyPath
	}
	return load(filter)
}

func usableNodes(nodes []Node, filter func([]Node) []Node) []Node {
	if filter == nil {
		return nodes
	}
	return filter(nodes)
}

func randomIndex(n int, pick func(int) (int, error)) (int, error) {
	if pick == nil {
		return 0, nil
	}
	return pick(n)
}

func loadUsableNode(nodeID string, load func(string) (Node, error)) (Node, error) {
	if load == nil {
		return Node{}, ErrNoUsableProxyPath
	}
	return load(nodeID)
}
