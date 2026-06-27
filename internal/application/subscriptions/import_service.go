package subscriptions

import (
	"context"

	appnodes "proxygateway/internal/application/nodes"
)

type ImportTx interface {
	NodeUpsertRepository() appnodes.UpsertRepository
	SubscriptionImportRepository() ImportRepository
	SubscriptionSourceRepository(nowMillis int64) SourceRepository
}

type ImportTxRunner interface {
	WithImportTx(ctx context.Context, fn func(ImportTx) error) error
}

type ImportCommand struct {
	SubscriptionID             string
	Name                       string
	SourceType                 string
	URL                        string
	Content                    string
	AutoRefreshEnabled         bool
	AutoRefreshIntervalSeconds int
	Refresh                    bool
	NowMillis                  int64
}

type ImportService struct {
	Runner    ImportTxRunner
	NewNodeID func() (string, error)
}

func (s ImportService) Import(ctx context.Context, command ImportCommand, parsed ParsedImportContent) (ImportResult, RefreshSnapshotResult, error) {
	nodeService := appnodes.Service{NewNodeID: s.NewNodeID}
	currentNodeIDs := make([]string, 0, len(parsed.Nodes))
	var snapshot RefreshSnapshotResult
	err := s.Runner.WithImportTx(ctx, func(tx ImportTx) error {
		importRecord := ImportRecord{
			ID:                         command.SubscriptionID,
			Name:                       command.Name,
			SourceType:                 command.SourceType,
			URL:                        command.URL,
			Content:                    command.Content,
			ImportedNodes:              len(parsed.Nodes),
			SkippedEntries:             parsed.SkippedEntries,
			SkippedSummaryJSON:         parsed.SkippedSummaryJSON,
			AutoRefreshEnabled:         command.AutoRefreshEnabled,
			AutoRefreshIntervalSeconds: command.AutoRefreshIntervalSeconds,
			NowMillis:                  command.NowMillis,
		}
		importRepo := tx.SubscriptionImportRepository()
		if command.Refresh {
			if err := importRepo.RefreshImport(importRecord); err != nil {
				return err
			}
		} else if err := importRepo.CreateImport(importRecord); err != nil {
			return err
		}
		for _, parsedNode := range parsed.Nodes {
			input, err := appnodes.BuildUpsertInput(outboundNodeFromParsed(parsedNode), appnodes.SourceInput{
				ID:   command.SubscriptionID,
				Name: command.Name,
				Type: "subscription",
			}, command.NowMillis)
			if err != nil {
				return err
			}
			nodeID, err := nodeService.Upsert(tx.NodeUpsertRepository(), input)
			if err != nil {
				return err
			}
			currentNodeIDs = append(currentNodeIDs, nodeID)
		}
		if !command.Refresh {
			return nil
		}
		result, err := PruneRefreshSnapshot(tx.SubscriptionSourceRepository(command.NowMillis), command.SubscriptionID, currentNodeIDs)
		if err != nil {
			return err
		}
		snapshot = result
		return nil
	})
	if err != nil {
		return ImportResult{}, RefreshSnapshotResult{}, err
	}
	return NewImportResult(command.SubscriptionID, len(parsed.Nodes), parsed.SkippedEntries, parsed.SkippedSummary, snapshot.StickyProfilesToEvaluate), snapshot, nil
}

func outboundNodeFromParsed(node ParsedNode) appnodes.OutboundNode {
	return appnodes.OutboundNode{
		Name:          node.Name,
		Type:          node.Type,
		Server:        node.Server,
		ServerPort:    node.ServerPort,
		Method:        node.Method,
		UUID:          node.UUID,
		Flow:          node.Flow,
		Security:      node.Security,
		AlterID:       node.AlterID,
		TLSJSON:       node.TLSJSON,
		TransportJSON: node.TransportJSON,
		Username:      node.Username,
		Password:      node.Password,
		RawJSON:       node.RawJSON,
		OutboundJSON:  node.OutboundJSON,
	}
}
