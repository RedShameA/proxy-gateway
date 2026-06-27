package sqlite

import (
	"context"
	"testing"

	appsettings "proxygateway/internal/application/settings"
)

func TestSystemSettingsRepositoryLoadSave(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repo := NewSystemSettingsRepository(db)
	ctx := context.Background()

	evaluation, err := repo.LoadEvaluation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.GlobalConcurrency <= 0 {
		t.Fatalf("default evaluation = %#v", evaluation)
	}
	evaluation.ConnectTimeoutSeconds = 11
	evaluation.ProbeTimeoutSeconds = 12
	if err := repo.SaveEvaluation(ctx, evaluation); err != nil {
		t.Fatal(err)
	}
	loadedEvaluation, err := repo.LoadEvaluation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loadedEvaluation.ConnectTimeoutSeconds != 11 || loadedEvaluation.ProbeTimeoutSeconds != 12 {
		t.Fatalf("loaded evaluation = %#v", loadedEvaluation)
	}

	maintenance := appsettings.MaintenanceSettings{
		SubscriptionRefreshSeconds:   100,
		NodeObservationSeconds:       101,
		ProfileEvaluationSeconds:     102,
		ChainEvaluationSeconds:       103,
		GeoIPUpdateTime:              "08:30",
		EgressIPProbeURL:             "https://example.test/trace",
		SubscriptionConcurrency:      2,
		NodeObservationConcurrency:   3,
		ProfileEvaluationConcurrency: 4,
		GeoIPConcurrency:             5,
	}
	if err := repo.SaveMaintenance(ctx, maintenance); err != nil {
		t.Fatal(err)
	}
	loadedMaintenance, err := repo.LoadMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loadedMaintenance != maintenance {
		t.Fatalf("loaded maintenance = %#v, want %#v", loadedMaintenance, maintenance)
	}
}
