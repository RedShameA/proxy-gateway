package app

import (
	"context"
	"errors"

	apperrors "proxygateway/internal/application/apperrors"
	appmaintenance "proxygateway/internal/application/maintenance"
	appsubscriptions "proxygateway/internal/application/subscriptions"
)

type skippedEntrySummarySet = appsubscriptions.SkippedEntrySummarySet
type subscriptionImportResult = appsubscriptions.ImportResult
type stickyProfileEvaluationRef = appsubscriptions.StickyProfileEvaluationRef

type subscriptionRecord = appsubscriptions.Record

type subscriptionCreateInput struct {
	Name                       string
	SourceType                 string
	URL                        string
	Content                    string
	AutoRefreshEnabled         *bool
	AutoRefreshIntervalSeconds int
}

type subscriptionFetchError struct {
	err error
}

func (err subscriptionFetchError) Error() string {
	return err.err.Error()
}

func (err subscriptionFetchError) Unwrap() error {
	return err.err
}

func newSubscriptionOperationError(kind, message string, err error) error {
	return apperrors.New(kind, message, err)
}

type subscriptionRefreshRunError struct {
	reasonCode string
	err        error
}

func (err subscriptionRefreshRunError) Error() string {
	return err.err.Error()
}

func (err subscriptionRefreshRunError) Unwrap() error {
	return err.err
}

var errInvalidSubscriptionContent = appsubscriptions.ErrInvalidContent

func (g *Gateway) createSubscriptionSource(input subscriptionCreateInput) (subscriptionImportResult, error) {
	result, err := g.subscriptionSourceService().Create(context.Background(), appsubscriptions.CreateSourceCommand{
		Name:                       input.Name,
		SourceType:                 input.SourceType,
		URL:                        input.URL,
		Content:                    input.Content,
		AutoRefreshEnabled:         input.AutoRefreshEnabled,
		AutoRefreshIntervalSeconds: input.AutoRefreshIntervalSeconds,
	})
	if err != nil {
		if errors.Is(err, appsubscriptions.ErrInvalidContent) {
			return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindBadRequest, err.Error(), err)
		}
		return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindBadGateway, err.Error(), subscriptionFetchError{err: err})
	}
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	return result.ImportResult, nil
}

func (g *Gateway) refreshSubscriptionSource(subscriptionID string) (subscriptionImportResult, error) {
	sub, err := g.loadSubscription(subscriptionID)
	if err != nil {
		if errors.Is(err, appsubscriptions.ErrSubscriptionNotFound) {
			return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindNotFound, "subscription not found", err)
		}
		return subscriptionImportResult{}, err
	}
	runID, err := g.enqueueSubscriptionRefreshRun(sub.ID, sub.Name, appmaintenance.TriggerManual)
	if err != nil {
		return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindInternal, err.Error(), err)
	}
	if err := g.runSubscriptionRefreshMaintenanceRun(runID); err != nil {
		reasonCode := ""
		if failedRun, loadErr := g.loadMaintenanceRun(runID); loadErr == nil {
			reasonCode = failedRun.ReasonCode
		}
		refreshErr := subscriptionRefreshRunError{reasonCode: reasonCode, err: err}
		switch {
		case errors.Is(err, errInvalidSubscriptionContent):
			return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindBadRequest, err.Error(), refreshErr)
		case reasonCode == appmaintenance.ReasonFetchFailed:
			return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindBadGateway, err.Error(), refreshErr)
		default:
			return subscriptionImportResult{}, newSubscriptionOperationError(apperrors.KindInternal, err.Error(), refreshErr)
		}
	}
	return g.subscriptionImportResult(sub.ID), nil
}

func (g *Gateway) subscriptionImportResult(subscriptionID string) subscriptionImportResult {
	record, found, err := g.subscriptionRepo.LoadImportResult(context.Background(), subscriptionID)
	if err != nil || !found {
		return subscriptionImportResult{}
	}
	return appsubscriptions.ImportResultFromRecord(record)
}

func (g *Gateway) deleteSubscriptionSource(subscriptionID string) error {
	result, err := appsubscriptions.DeleteService{
		Runner: g.txRunners,
		Now:    unixMillisNow,
	}.Delete(context.Background(), subscriptionID)
	if errors.Is(err, appsubscriptions.ErrSubscriptionNotFound) {
		return errors.New("subscription not found")
	}
	if err != nil {
		return err
	}
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	g.notifyMaintenanceRunner()
	return nil
}

func (g *Gateway) loadSubscription(id string) (subscriptionRecord, error) {
	record, found, err := g.subscriptionRepo.Load(context.Background(), id)
	if err != nil {
		return subscriptionRecord{}, err
	}
	if !found {
		return subscriptionRecord{}, appsubscriptions.ErrSubscriptionNotFound
	}
	return record, nil
}

func (g *Gateway) updateSubscriptionAutoRefresh(subscriptionID string, enabled *bool, intervalSeconds *int) error {
	updated, err := g.subscriptionRepo.UpdateAutoRefresh(context.Background(), subscriptionID, enabled, intervalSeconds, unixMillisNow())
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("subscription not found")
	}
	return nil
}

func (g *Gateway) subscriptionContentForImport(sub subscriptionRecord) (string, error) {
	content, err := g.subscriptionSourceService().ContentForImport(context.Background(), sub)
	if errors.Is(err, appsubscriptions.ErrRemoteSubscriptionURLRequired) {
		return "", errors.New(validationSubscriptionURLRequired)
	}
	return content, err
}

func (g *Gateway) createSubscriptionWithContent(sub subscriptionRecord, content string) (subscriptionImportResult, error) {
	return g.importSubscriptionWithContent(sub, content, false)
}

func (g *Gateway) refreshSubscriptionWithContent(sub subscriptionRecord, content string) (subscriptionImportResult, error) {
	return g.importSubscriptionWithContent(sub, content, true)
}

func (g *Gateway) importSubscriptionWithContent(sub subscriptionRecord, content string, refresh bool) (subscriptionImportResult, error) {
	result, err := g.subscriptionSourceService().ImportWithContent(context.Background(), sub, content, refresh)
	if err != nil {
		return subscriptionImportResult{}, err
	}
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	return result.ImportResult, nil
}

func (g *Gateway) subscriptionSourceService() appsubscriptions.SourceService {
	return appsubscriptions.SourceService{
		Runner:  g.txRunners,
		Fetcher: g.subscriptionFetcher,
		NewID: func(prefix string) (string, error) {
			return prefixedID(prefix)
		},
		Now: unixMillisNow,
	}
}

func (g *Gateway) enqueueStickyProfileEvaluationsForRemovedNodes(refs []stickyProfileEvaluationRef) {
	for _, ref := range refs {
		_, _ = g.enqueueProfileEvaluationRun(ref.ID, ref.Name, appmaintenance.TriggerCurrentNodeRemoved, ref.ConfigVersion, true)
	}
}

func (g *Gateway) profileRetainsNode(profileID, nodeID string) bool {
	return g.profileRetainsNodeWithContext(context.Background(), profileID, nodeID)
}

func (g *Gateway) profileRetainsNodeWithContext(ctx context.Context, profileID, nodeID string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if g.evaluationRepo == nil {
		return false
	}
	exists, err := g.evaluationRepo.ProfileRetainsNode(ctx, profileID, nodeID)
	return err == nil && exists
}

func (g *Gateway) enqueueProfileEvaluationsWaitingForObservation() {
	profiles, err := g.maintenanceAuxRepo.ListProfilesWaitingForObservation(context.Background())
	if err != nil {
		return
	}
	for _, profile := range profiles {
		if g.hasUnfinishedCurrentNodeObservedEvaluation(profile.ID) {
			continue
		}
		_, _ = g.enqueueProfileEvaluationRun(profile.ID, profile.Name, appmaintenance.TriggerCurrentNodeObserved, profile.ConfigVersion, true)
	}
}

func (g *Gateway) hasUnfinishedCurrentNodeObservedEvaluation(profileID string) bool {
	exists, err := g.maintenanceAuxRepo.HasUnfinishedCurrentNodeObservedEvaluation(context.Background(), profileID)
	return err == nil && exists
}

func (g *Gateway) storeSubscriptionRefreshError(subscriptionID, errorText string) {
	_ = g.subscriptionRepo.StoreRefreshError(context.Background(), subscriptionID, errorText, unixMillisNow())
}
