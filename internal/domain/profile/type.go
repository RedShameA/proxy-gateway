package profile

import (
	"errors"
	"strings"
)

var (
	ErrExitNodesRequired           = errors.New("exit nodes required")
	ErrChainLinkSingleExitRequired = errors.New("chain link requires single exit node")
)

func NormalizeChainEvaluationMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == ChainEvaluationModeChainLink {
		return ChainEvaluationModeChainLink
	}
	return ChainEvaluationModeEndToEnd
}

func ValidateChainExitNodes(exitNodeIDs []string, chainEvaluationMode string) error {
	if len(exitNodeIDs) == 0 {
		return ErrExitNodesRequired
	}
	if NormalizeChainEvaluationMode(chainEvaluationMode) == ChainEvaluationModeChainLink && len(exitNodeIDs) != 1 {
		return ErrChainLinkSingleExitRequired
	}
	return nil
}
