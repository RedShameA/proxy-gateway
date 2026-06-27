package subscriptions

import (
	"context"
	"errors"
)

var ErrInvalidContent = errors.New("invalid subscription content")

type SourceIDGenerator func(prefix string) (string, error)
type SourceClock func() int64

type SourceService struct {
	Runner  ImportTxRunner
	Fetcher ContentFetcher
	NewID   SourceIDGenerator
	Now     SourceClock
}

type CreateSourceCommand struct {
	Name                       string
	SourceType                 string
	URL                        string
	Content                    string
	AutoRefreshEnabled         *bool
	AutoRefreshIntervalSeconds int
}

type SourceImportResult struct {
	ImportResult        ImportResult
	DeletedFingerprints []string
}

func (s SourceService) Create(ctx context.Context, command CreateSourceCommand) (SourceImportResult, error) {
	autoRefreshEnabled := true
	if command.AutoRefreshEnabled != nil {
		autoRefreshEnabled = *command.AutoRefreshEnabled
	}
	id, err := s.newID("sub")
	if err != nil {
		return SourceImportResult{}, err
	}
	record := Record{
		ID:                         id,
		Name:                       command.Name,
		SourceType:                 command.SourceType,
		URL:                        command.URL,
		Content:                    command.Content,
		AutoRefreshEnabled:         autoRefreshEnabled,
		AutoRefreshIntervalSeconds: command.AutoRefreshIntervalSeconds,
	}
	content, err := s.ContentForImport(ctx, record)
	if err != nil {
		return SourceImportResult{}, err
	}
	return s.ImportWithContent(ctx, record, content, false)
}

func (s SourceService) ContentForImport(ctx context.Context, record Record) (string, error) {
	return ContentForImport(ctx, ContentSource{
		SourceType: record.SourceType,
		URL:        record.URL,
		Content:    record.Content,
	}, s.Fetcher)
}

func (s SourceService) ImportWithContent(ctx context.Context, record Record, content string, refresh bool) (SourceImportResult, error) {
	parsed, err := ParseImportContent(content)
	if err != nil {
		return SourceImportResult{}, errors.Join(ErrInvalidContent, err)
	}
	result, snapshot, err := ImportService{
		Runner: s.Runner,
		NewNodeID: func() (string, error) {
			return s.newID("node")
		},
	}.Import(ctx, ImportCommand{
		SubscriptionID:             record.ID,
		Name:                       record.Name,
		SourceType:                 record.SourceType,
		URL:                        record.URL,
		Content:                    content,
		AutoRefreshEnabled:         record.AutoRefreshEnabled,
		AutoRefreshIntervalSeconds: record.AutoRefreshIntervalSeconds,
		Refresh:                    refresh,
		NowMillis:                  s.now(),
	}, parsed)
	if err != nil {
		return SourceImportResult{}, err
	}
	return SourceImportResult{
		ImportResult:        result,
		DeletedFingerprints: snapshot.DeletedFingerprints,
	}, nil
}

func (s SourceService) newID(prefix string) (string, error) {
	if s.NewID == nil {
		return "", errors.New("subscription source id generator is nil")
	}
	return s.NewID(prefix)
}

func (s SourceService) now() int64 {
	if s.Now == nil {
		return 0
	}
	return s.Now()
}
