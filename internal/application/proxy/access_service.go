package proxy

import (
	"context"
	"errors"

	appprofiles "proxygateway/internal/application/profiles"
)

var ErrAccessProfileConfigNotFound = errors.New("access profile not found")

type ProfileConfigLoader func(ctx context.Context, profileID string) (appprofiles.ConfigRecord, error)

type AccessService struct {
	Credentials       CredentialRepository
	LoadProfileConfig ProfileConfigLoader
	Path              PathSelectionDeps
	NowMillis         func() int64
	TouchWindowMillis int64
}

func (s AccessService) Authenticate(ctx context.Context, username, password string) CredentialAuthResult {
	return AuthenticateCredential(ctx, s.Credentials, username, password, s.nowMillis(), s.touchWindowMillis())
}

func (s AccessService) SelectPath(ctx context.Context, credential CredentialRecord) (SelectedPath, error) {
	if s.LoadProfileConfig == nil {
		return SelectedPath{}, ErrAccessProfileConfigNotFound
	}
	cfg, err := s.LoadProfileConfig(ctx, credential.ProfileID)
	if err != nil {
		return SelectedPath{}, ErrAccessProfileConfigNotFound
	}
	return SelectPathForCredential(credential, cfg, s.Path)
}

func (s AccessService) nowMillis() int64 {
	if s.NowMillis == nil {
		return 0
	}
	return s.NowMillis()
}

func (s AccessService) touchWindowMillis() int64 {
	if s.TouchWindowMillis <= 0 {
		return 60_000
	}
	return s.TouchWindowMillis
}
