package subscriptions

import "context"

type Record struct {
	ID                         string
	Name                       string
	SourceType                 string
	URL                        string
	Content                    string
	ImportedNodes              int
	SkippedEntries             int
	SkippedSummaryJSON         string
	LastError                  string
	AutoRefreshEnabled         bool
	AutoRefreshIntervalSeconds int
	UpdatedAt                  int64
}

type ListFilter struct {
	Limit  int
	Offset int
}

type ListResult struct {
	Items []Record
	Total int
}

type ImportResultRecord struct {
	ID                 string
	ImportedNodes      int
	SkippedEntries     int
	SkippedSummaryJSON string
}

type Repository interface {
	List(ctx context.Context, filter ListFilter) (ListResult, error)
	Load(ctx context.Context, id string) (Record, bool, error)
	LoadImportResult(ctx context.Context, id string) (ImportResultRecord, bool, error)
	UpdateAutoRefresh(ctx context.Context, id string, enabled *bool, intervalSeconds *int, updatedAt int64) (bool, error)
	StoreRefreshError(ctx context.Context, id, errorText string, updatedAt int64) error
}

type SourceRepository interface {
	DeleteRepository
	RefreshSnapshotRepository
}
