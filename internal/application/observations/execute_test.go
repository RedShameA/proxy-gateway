package observations

import (
	"errors"
	"testing"
)

func TestExecuteNodeObservationPersistsSuccessfulProbeAndReturnsSuccessfulRunResult(t *testing.T) {
	repo := &fakePersistenceRepository{}
	lookup := fakeCountryLookup{countryByIP: map[string]string{
		"198.51.100.20": "US",
	}}
	executor := fakeProbeExecutor{
		payload: ProbePayload{
			Raw:       []byte("ip=198.51.100.20\nloc=sg\n"),
			LatencyMS: 37,
		},
	}

	result := ExecuteNodeObservation(repo, lookup, executor, NodeTarget{ID: "node-1", Name: "Node 1"}, 1234)

	if !result.OK || result.NodeID != "node-1" || result.Name != "Node 1" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if repo.successNodeID != "node-1" || repo.successObservedAt != 1234 {
		t.Fatalf("success identity = %#v", repo)
	}
	if repo.successRecord.EgressIP != "198.51.100.20" || repo.successRecord.EgressCountry != "US" || repo.successRecord.LatencyMS != 37 {
		t.Fatalf("success record = %#v", repo.successRecord)
	}
}

func TestExecuteNodeObservationPersistsFailedProbeAndReturnsFailedRunResult(t *testing.T) {
	repo := &fakePersistenceRepository{}
	executor := fakeProbeExecutor{err: errors.New("dial failed")}

	result := ExecuteNodeObservation(repo, nil, executor, NodeTarget{ID: "node-2", Name: "Node 2"}, 5678)

	if result.OK || result.NodeID != "node-2" || result.Name != "Node 2" || result.Error != "dial failed" {
		t.Fatalf("result = %#v", result)
	}
	if repo.failureNodeID != "node-2" || repo.failureError != "dial failed" || repo.failureObservedAt != 5678 {
		t.Fatalf("failure payload = %#v", repo)
	}
}

type fakeProbeExecutor struct {
	payload ProbePayload
	err     error
}

func (f fakeProbeExecutor) Probe() (ProbePayload, error) {
	return f.payload, f.err
}
