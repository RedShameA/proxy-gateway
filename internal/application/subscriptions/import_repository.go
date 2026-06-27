package subscriptions

type ImportRecord struct {
	ID                         string
	Name                       string
	SourceType                 string
	URL                        string
	Content                    string
	ImportedNodes              int
	SkippedEntries             int
	SkippedSummaryJSON         string
	AutoRefreshEnabled         bool
	AutoRefreshIntervalSeconds int
	NowMillis                  int64
}

type ImportRepository interface {
	CreateImport(record ImportRecord) error
	RefreshImport(record ImportRecord) error
}
