package observations

import (
	"errors"
	"testing"
)

func TestExecuteBatchRunsAllTargetsWithBoundedConcurrency(t *testing.T) {
	repo := &fakePersistenceRepository{}
	targets := []ExecutableTarget{
		{
			Target: NodeTarget{ID: "node-1", Name: "Node 1"},
			Executor: fakeProbeExecutor{payload: ProbePayload{
				Raw:       []byte("ip=198.51.100.20\nloc=sg\n"),
				LatencyMS: 11,
			}},
		},
		{
			Target:   NodeTarget{ID: "node-2", Name: "Node 2"},
			Executor: fakeProbeExecutor{err: errors.New("dial failed")},
		},
	}

	results := ExecuteBatch(repo, nil, targets, 1, func() int64 {
		return 1234
	})

	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2: %#v", len(results), results)
	}
	byID := map[string]RunResult{}
	for _, result := range results {
		byID[result.NodeID] = result
	}
	if !byID["node-1"].OK || byID["node-2"].Error != "dial failed" {
		t.Fatalf("results = %#v", byID)
	}
	if repo.successNodeID != "node-1" || repo.failureNodeID != "node-2" {
		t.Fatalf("repo writes = %#v", repo)
	}
}
