package app

import (
	"context"
	"errors"
	"testing"

	appmaintenance "proxygateway/internal/application/maintenance"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestCreateSubscriptionWarnsWhenImportObservationEnqueueFails(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.DebugLevel)
	g := NewForTest(t, WithLogger(zap.New(core)), WithoutMaintenanceRunner())
	g.maintenanceAuxRepo = failingSubscriptionObservationAuxiliaryRepository{err: errors.New("observation target query failed")}

	result, err := g.createSubscriptionSource(subscriptionCreateInput{
		Name:       "enqueue-failure-create",
		SourceType: "local",
		Content: `{"outbounds":[
			{"type":"http","tag":"enqueue-failure-node","server":"127.0.0.1","server_port":18080}
		]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == "" || result.ImportedNodes != 1 {
		t.Fatalf("subscription result = %#v, want imported subscription", result)
	}

	assertSubscriptionObservationEnqueueWarn(t, observed, result.ID, appmaintenance.TriggerSubscriptionImport)
}

func TestRefreshSubscriptionWarnsWhenObservationEnqueueFailsAndStillSucceeds(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.DebugLevel)
	g := NewForTest(t, WithLogger(zap.New(core)), WithoutMaintenanceRunner())
	created, err := g.createSubscriptionSource(subscriptionCreateInput{
		Name:       "enqueue-failure-refresh",
		SourceType: "local",
		Content: `{"outbounds":[
			{"type":"http","tag":"refresh-enqueue-failure-node","server":"127.0.0.1","server_port":18080}
		]}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	g.maintenanceAuxRepo = failingSubscriptionObservationAuxiliaryRepository{err: errors.New("observation target query failed")}
	refreshed, err := g.refreshSubscriptionSource(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ID != created.ID || refreshed.ImportedNodes != 1 {
		t.Fatalf("refresh result = %#v, want successful refresh for %s", refreshed, created.ID)
	}

	assertSubscriptionObservationEnqueueWarn(t, observed, created.ID, appmaintenance.TriggerSubscriptionRefresh)
	run := latestMaintenanceRunByTypeForTest(t, g, maintenanceTaskSubscriptionRefresh)
	if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultSuccess || run.ReasonCode != maintenanceRunReasonCompleted {
		t.Fatalf("refresh run = %#v, want finished success completed", run)
	}
}

func assertSubscriptionObservationEnqueueWarn(t *testing.T, observed *observer.ObservedLogs, subscriptionID, triggerSource string) {
	t.Helper()

	logs := observed.FilterMessage("enqueue subscription node observation failed").All()
	for _, entry := range logs {
		fields := entry.ContextMap()
		if fields["subscription_id"] != subscriptionID || fields["trigger_source"] != triggerSource {
			continue
		}
		if entry.Level != zapcore.WarnLevel {
			t.Fatalf("log level = %s, want warn", entry.Level)
		}
		if _, ok := fields["error"]; !ok {
			t.Fatalf("warn fields = %#v, want error field", fields)
		}
		return
	}
	t.Fatalf("warn logs = %#v, want subscription_id=%s trigger_source=%s", logs, subscriptionID, triggerSource)
}

type failingSubscriptionObservationAuxiliaryRepository struct {
	err error
}

func (r failingSubscriptionObservationAuxiliaryRepository) ListNodeObservationScheduleTargets(context.Context) ([]appmaintenance.NodeObservationScheduleTarget, error) {
	return nil, nil
}

func (r failingSubscriptionObservationAuxiliaryRepository) ListSubscriptionNodeObservationTargets(context.Context, string) ([]appmaintenance.NodeObservationScheduleTarget, error) {
	return nil, r.err
}

func (r failingSubscriptionObservationAuxiliaryRepository) ListProfileEvaluationScheduleTargets(context.Context) ([]appmaintenance.ProfileEvaluationScheduleTarget, error) {
	return nil, nil
}

func (r failingSubscriptionObservationAuxiliaryRepository) ListProfilesWaitingForObservation(context.Context) ([]appmaintenance.WaitingObservationProfile, error) {
	return nil, nil
}

func (r failingSubscriptionObservationAuxiliaryRepository) ListSubscriptionRefreshScheduleTargets(context.Context) ([]appmaintenance.SubscriptionRefreshScheduleTarget, error) {
	return nil, nil
}

func (r failingSubscriptionObservationAuxiliaryRepository) HasRecentRun(context.Context, string, int64) (bool, error) {
	return false, nil
}

func (r failingSubscriptionObservationAuxiliaryRepository) HasUnfinishedCurrentNodeObservedEvaluation(context.Context, string) (bool, error) {
	return false, nil
}

func (r failingSubscriptionObservationAuxiliaryRepository) DeleteHistoryBefore(context.Context, int64, string) (int64, error) {
	return 0, nil
}
