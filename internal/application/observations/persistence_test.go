package observations

import "testing"

func TestPersistSuccessBuildsRecordWithGeoIPLookupAndStoresIt(t *testing.T) {
	repo := &fakePersistenceRepository{}
	lookup := fakeCountryLookup{countryByIP: map[string]string{
		"198.51.100.20": "US",
	}}

	err := PersistSuccess(repo, lookup, "node-1", []byte("ip=198.51.100.20\nloc=sg\n"), 37, 1234)
	if err != nil {
		t.Fatalf("PersistSuccess error = %v", err)
	}
	if repo.successNodeID != "node-1" || repo.successObservedAt != 1234 {
		t.Fatalf("success identity = %#v", repo)
	}
	if repo.successRecord.EgressIP != "198.51.100.20" || repo.successRecord.EgressCountry != "US" || repo.successRecord.LatencyMS != 37 {
		t.Fatalf("success record = %#v", repo.successRecord)
	}
}

func TestPersistFailureStoresFailureTimestampAndError(t *testing.T) {
	repo := &fakePersistenceRepository{}

	err := PersistFailure(repo, "node-2", "dial failed", 5678)
	if err != nil {
		t.Fatalf("PersistFailure error = %v", err)
	}
	if repo.failureNodeID != "node-2" || repo.failureError != "dial failed" || repo.failureObservedAt != 5678 {
		t.Fatalf("failure payload = %#v", repo)
	}
}

type fakePersistenceRepository struct {
	successNodeID     string
	successRecord     SuccessRecord
	successObservedAt int64
	failureNodeID     string
	failureError      string
	failureObservedAt int64
}

func (f *fakePersistenceRepository) SaveSuccess(nodeID string, record SuccessRecord, observedAt int64) error {
	f.successNodeID = nodeID
	f.successRecord = record
	f.successObservedAt = observedAt
	return nil
}

func (f *fakePersistenceRepository) SaveFailure(nodeID, errorText string, observedAt int64) error {
	f.failureNodeID = nodeID
	f.failureError = errorText
	f.failureObservedAt = observedAt
	return nil
}

type fakeCountryLookup struct {
	countryByIP map[string]string
}

func (f fakeCountryLookup) LookupCountry(ip string) string {
	return f.countryByIP[ip]
}
