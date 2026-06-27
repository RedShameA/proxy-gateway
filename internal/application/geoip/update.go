package geoip

import (
	"context"
	"errors"

	appmaintenance "proxygateway/internal/application/maintenance"
)

const SourceMetaCubeX = "MetaCubeX"

var ErrServiceUnavailable = errors.New("geoip service not available")

type Updater interface {
	UpdateFromMetaCubeXLatest() error
}

type StatusSnapshotFunc func() any

type UpdateService struct {
	Updater Updater
	Status  StatusSnapshotFunc
}

func (s UpdateService) Execute(_ context.Context, run appmaintenance.Run) (appmaintenance.FinishCommand, error) {
	detail := appmaintenance.RunDetail(run)
	detail["source"] = SourceMetaCubeX
	if s.Updater == nil {
		detail["geoip"] = s.status()
		return appmaintenance.FinishCommand{
			ID:            run.ID,
			Result:        appmaintenance.ResultFailure,
			ReasonCode:    "geoip_service_unavailable",
			FinishedCount: 1,
			Detail:        detail,
			LastError:     ErrServiceUnavailable.Error(),
		}, ErrServiceUnavailable
	}
	err := s.Updater.UpdateFromMetaCubeXLatest()
	detail["geoip"] = s.status()
	if err != nil {
		return appmaintenance.FinishCommand{
			ID:            run.ID,
			Result:        appmaintenance.ResultFailure,
			ReasonCode:    "geoip_update_failed",
			FinishedCount: 1,
			Detail:        detail,
			LastError:     err.Error(),
		}, err
	}
	return appmaintenance.FinishCommand{
		ID:            run.ID,
		Result:        appmaintenance.ResultSuccess,
		ReasonCode:    appmaintenance.ReasonCompleted,
		FinishedCount: 1,
		Detail:        detail,
	}, nil
}

func (s UpdateService) status() any {
	if s.Status == nil {
		return nil
	}
	return s.Status()
}
