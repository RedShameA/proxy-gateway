package subscriptions

import (
	"context"
	"errors"

	appnodes "proxygateway/internal/application/nodes"
)

var ErrNoImportableNodeFound = errors.New("no importable node found")

type ManualNodeImportTx interface {
	NodeUpsertRepository() appnodes.UpsertRepository
}

type ManualNodeImportTxRunner interface {
	WithManualNodeImportTx(ctx context.Context, fn func(ManualNodeImportTx) error) error
}

type ManualNodeImportCommand struct {
	ImportText string
	NowMillis  int64
}

type ManualImportedNode struct {
	ID   string
	Name string
}

type ManualNodeImportResult struct {
	Nodes               []ManualImportedNode
	SkippedEntries      int
	SkippedEntrySummary []SkippedEntrySummary
}

type ManualNodeImportService struct {
	Runner    ManualNodeImportTxRunner
	NewNodeID func() (string, error)
}

func (s ManualNodeImportService) Import(ctx context.Context, command ManualNodeImportCommand) (ManualNodeImportResult, error) {
	parsed, skippedSummary, err := ParseManualNodeImport(command.ImportText)
	if err != nil {
		return ManualNodeImportResult{}, err
	}
	if len(parsed) == 0 {
		return ManualNodeImportResult{}, ErrNoImportableNodeFound
	}
	nodeService := appnodes.Service{NewNodeID: s.NewNodeID}
	nodes := make([]ManualImportedNode, 0, len(parsed))
	err = s.Runner.WithManualNodeImportTx(ctx, func(tx ManualNodeImportTx) error {
		for _, parsedNode := range parsed {
			input, err := appnodes.BuildUpsertInput(outboundNodeFromParsed(parsedNode), appnodes.SourceInput{
				ID:   appnodes.SourceTypeManual,
				Name: "Manual",
				Type: appnodes.SourceTypeManual,
			}, command.NowMillis)
			if err != nil {
				return err
			}
			nodeID, err := nodeService.Upsert(tx.NodeUpsertRepository(), input)
			if err != nil {
				return err
			}
			nodes = append(nodes, ManualImportedNode{ID: nodeID, Name: parsedNode.Name})
		}
		return nil
	})
	if err != nil {
		return ManualNodeImportResult{}, err
	}
	return ManualNodeImportResult{
		Nodes:               nodes,
		SkippedEntries:      skippedSummary.Count(),
		SkippedEntrySummary: skippedSummary.Rows(),
	}, nil
}
