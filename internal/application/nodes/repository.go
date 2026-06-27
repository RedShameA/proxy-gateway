package nodes

import "context"

type Record struct {
	ID           string
	Name         string
	Type         string
	Server       string
	ServerPort   int
	Username     string
	Password     string
	RawJSON      string
	OutboundJSON string
	Enabled      bool
}

type SourceRecord struct {
	SourceID    string
	SourceName  string
	SourceType  string
	DisplayName string
}

type ObservationRecord struct {
	Usable        bool
	EgressIP      string
	EgressCountry string
	LatencyMS     int64
	LastError     string
	LastSuccessAt int64
	LastFailureAt int64
}

type ListFilter struct {
	Name          string
	EgressCountry string
	Protocol      string
	SourceID      string
	SourceType    string
	State         string
	Usable        *bool
	Limit         int
	Offset        int
}

type ListResult struct {
	IDs   []string
	Total int
}

type Repository interface {
	Load(ctx context.Context, id string) (Record, bool, error)
	ListIDs(ctx context.Context, filter ListFilter) (ListResult, error)
	ListEnabledObservationTargets(ctx context.Context) ([]Record, error)
	ListSources(ctx context.Context, nodeID string) ([]SourceRecord, error)
	LoadObservation(ctx context.Context, nodeID string) (ObservationRecord, bool, error)
	SetEnabled(ctx context.Context, nodeID string, enabled bool) (bool, error)
}
