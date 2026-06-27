package nodes

import "context"

type ReadService struct {
	repo Repository
}

func NewReadService(repo Repository) ReadService {
	return ReadService{repo: repo}
}

func (s ReadService) List(ctx context.Context, filter ListFilter) (map[string]any, error) {
	result, err := s.repo.ListIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	nodes := make([]map[string]any, 0, len(result.IDs))
	for _, nodeID := range result.IDs {
		item, err := s.ListItem(ctx, nodeID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, item)
	}
	return map[string]any{"items": nodes, "nodes": nodes, "total": result.Total}, nil
}

func (s ReadService) Detail(ctx context.Context, nodeID string) (map[string]any, error) {
	node, found, err := s.repo.Load(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNodeNotFound
	}
	return BuildDetail(node, s.sourceRecords(ctx, nodeID), s.observationSnapshot(ctx, nodeID)), nil
}

func (s ReadService) ListItem(ctx context.Context, nodeID string) (map[string]any, error) {
	node, found, err := s.repo.Load(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNodeNotFound
	}
	return BuildListItem(node, s.sourceRecords(ctx, node.ID), s.observationSnapshot(ctx, node.ID)), nil
}

func (s ReadService) sourceRecords(ctx context.Context, nodeID string) []SourceRecord {
	sources, err := s.repo.ListSources(ctx, nodeID)
	if err != nil {
		return nil
	}
	return sources
}

func (s ReadService) observationSnapshot(ctx context.Context, nodeID string) ObservationSnapshot {
	observation, found, err := s.repo.LoadObservation(ctx, nodeID)
	if err != nil {
		return UnavailableObservationSnapshot()
	}
	return ObservationSnapshotFromRecord(observation, found)
}
