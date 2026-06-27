package nodes

import "errors"

var (
	ErrNodeNotFound            = errors.New("node not found")
	ErrManualNodeSourceMissing = errors.New("manual node source not found")
	ErrDuplicateNode           = errors.New("duplicate node")
)

type ManualUpdateRepository interface {
	UpsertRepository
	CurrentNodeEnabled(nodeID string) (int, error)
	NodeSourceCounts(nodeID string) (manualSources, totalSources int, err error)
	FindOtherNodeIDByFingerprint(fingerprint, excludeNodeID string) (string, error)
	UpdateNode(record UpdateNodeRecord) error
	UpdateManualSourceDisplayName(nodeID, name string) error
	DeleteManualSource(nodeID string) error
	SetNodeEnabled(nodeID string, enabled int) error
}

type ManualUpdateInput struct {
	NodeID       string
	Fingerprint  string
	Name         string
	Type         string
	Server       string
	ServerPort   int
	Username     string
	Password     string
	RawJSON      string
	OutboundJSON string
	Enabled      *bool
	NowMillis    int64
}

type UpdateNodeRecord struct {
	NodeID       string
	Fingerprint  string
	Name         string
	Type         string
	Server       string
	ServerPort   int
	Username     string
	Password     string
	RawJSON      string
	OutboundJSON string
	Enabled      int
}

type ManualUpdateResult struct {
	NodeID string
	Split  bool
}

func (s Service) UpdateManual(repo ManualUpdateRepository, input ManualUpdateInput) (ManualUpdateResult, error) {
	currentEnabled, err := repo.CurrentNodeEnabled(input.NodeID)
	if err != nil {
		return ManualUpdateResult{}, err
	}
	manualSources, totalSources, err := repo.NodeSourceCounts(input.NodeID)
	if err != nil {
		return ManualUpdateResult{}, err
	}
	if manualSources == 0 {
		return ManualUpdateResult{}, ErrManualNodeSourceMissing
	}
	enabled := currentEnabled
	if input.Enabled != nil {
		enabled = boolInt(*input.Enabled)
	}
	if totalSources <= manualSources {
		duplicateID, err := repo.FindOtherNodeIDByFingerprint(input.Fingerprint, input.NodeID)
		if err != nil {
			return ManualUpdateResult{}, err
		}
		if duplicateID != "" {
			return ManualUpdateResult{}, ErrDuplicateNode
		}
		if err := repo.UpdateNode(UpdateNodeRecord{
			NodeID:       input.NodeID,
			Fingerprint:  input.Fingerprint,
			Name:         input.Name,
			Type:         input.Type,
			Server:       input.Server,
			ServerPort:   input.ServerPort,
			Username:     input.Username,
			Password:     input.Password,
			RawJSON:      input.RawJSON,
			OutboundJSON: input.OutboundJSON,
			Enabled:      enabled,
		}); err != nil {
			return ManualUpdateResult{}, err
		}
		if err := repo.UpdateManualSourceDisplayName(input.NodeID, input.Name); err != nil {
			return ManualUpdateResult{}, err
		}
		return ManualUpdateResult{NodeID: input.NodeID}, nil
	}

	if err := repo.DeleteManualSource(input.NodeID); err != nil {
		return ManualUpdateResult{}, err
	}
	updatedNodeID, err := s.Upsert(repo, UpsertInput{
		Fingerprint:  input.Fingerprint,
		Name:         input.Name,
		Type:         input.Type,
		Server:       input.Server,
		ServerPort:   input.ServerPort,
		Username:     input.Username,
		Password:     input.Password,
		RawJSON:      input.RawJSON,
		OutboundJSON: input.OutboundJSON,
		SourceID:     SourceTypeManual,
		SourceName:   "Manual",
		SourceType:   SourceTypeManual,
		NowMillis:    input.NowMillis,
	})
	if err != nil {
		return ManualUpdateResult{}, err
	}
	if input.Enabled != nil {
		if err := repo.SetNodeEnabled(updatedNodeID, enabled); err != nil {
			return ManualUpdateResult{}, err
		}
	}
	return ManualUpdateResult{NodeID: updatedNodeID, Split: updatedNodeID != input.NodeID}, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
