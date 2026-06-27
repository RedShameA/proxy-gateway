package geoip

import (
	"context"
	"errors"
	"testing"

	appmaintenance "proxygateway/internal/application/maintenance"
)

func TestUpdateServiceReportsUnavailableService(t *testing.T) {
	finish, err := (UpdateService{Status: func() any {
		return map[string]any{"loaded": false}
	}}).Execute(context.Background(), appmaintenance.Run{ID: "run_geoip"})

	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("Execute error = %v, want ErrServiceUnavailable", err)
	}
	if finish.Result != appmaintenance.ResultFailure || finish.ReasonCode != "geoip_service_unavailable" {
		t.Fatalf("finish = %#v", finish)
	}
	if finish.Detail["source"] != SourceMetaCubeX || finish.Detail["geoip"] == nil {
		t.Fatalf("detail = %#v", finish.Detail)
	}
}

func TestUpdateServiceReportsUpdateFailure(t *testing.T) {
	updateErr := errors.New("download failed")
	finish, err := (UpdateService{
		Updater: fakeGeoIPUpdater{err: updateErr},
		Status: func() any {
			return map[string]any{"last_error": updateErr.Error()}
		},
	}).Execute(context.Background(), appmaintenance.Run{ID: "run_geoip"})

	if !errors.Is(err, updateErr) {
		t.Fatalf("Execute error = %v, want updateErr", err)
	}
	if finish.Result != appmaintenance.ResultFailure || finish.ReasonCode != "geoip_update_failed" || finish.LastError != updateErr.Error() {
		t.Fatalf("finish = %#v", finish)
	}
}

func TestUpdateServiceReportsSuccess(t *testing.T) {
	finish, err := (UpdateService{
		Updater: fakeGeoIPUpdater{},
		Status: func() any {
			return map[string]any{"loaded": true}
		},
	}).Execute(context.Background(), appmaintenance.Run{ID: "run_geoip"})

	if err != nil {
		t.Fatal(err)
	}
	if finish.Result != appmaintenance.ResultSuccess || finish.ReasonCode != appmaintenance.ReasonCompleted || finish.FinishedCount != 1 {
		t.Fatalf("finish = %#v", finish)
	}
}

type fakeGeoIPUpdater struct {
	err error
}

func (u fakeGeoIPUpdater) UpdateFromMetaCubeXLatest() error {
	return u.err
}
