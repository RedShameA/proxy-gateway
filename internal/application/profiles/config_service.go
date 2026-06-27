package profiles

import (
	"context"
	"errors"
	"fmt"
)

var ErrConfigServiceMissingDependency = errors.New("profile config service missing dependency")
var ErrConfigIDGeneration = errors.New("profile config id generation failed")
var ErrConfigCreateFailed = errors.New("profile config create failed")
var ErrConfigUpdateFailed = errors.New("profile config update failed")

type ConfigIDGenerator func() (string, error)
type ConfigClock func() int64

type ConfigUpdateWithRelease func(ctx context.Context, profileID string, record ConfigRecord, options ConfigUpdateOptions) ([]string, error)

type ConfigServiceDeps struct {
	Repository        ConfigRepository
	NewID             ConfigIDGenerator
	Now               ConfigClock
	Validation        ConfigValidationDeps
	UpdateWithRelease ConfigUpdateWithRelease
}

type ConfigService struct {
	repo              ConfigRepository
	newID             ConfigIDGenerator
	now               ConfigClock
	validation        ConfigValidationDeps
	updateWithRelease ConfigUpdateWithRelease
}

type ConfigMutationResult struct {
	Config                           ConfigRecord
	DeletedFingerprints              []string
	EnqueueEvaluation                bool
	EnqueueUnknownCountryObservation bool
}

func NewConfigService(deps ConfigServiceDeps) ConfigService {
	return ConfigService{
		repo:              deps.Repository,
		newID:             deps.NewID,
		now:               deps.Now,
		validation:        deps.Validation,
		updateWithRelease: deps.UpdateWithRelease,
	}
}

func (s ConfigService) Create(ctx context.Context, req PatchRequest) (ConfigMutationResult, error) {
	if s.repo == nil || s.newID == nil || s.now == nil {
		return ConfigMutationResult{}, ErrConfigServiceMissingDependency
	}
	id, err := s.newID()
	if err != nil {
		return ConfigMutationResult{}, fmt.Errorf("%w: %w", ErrConfigIDGeneration, err)
	}
	plan, err := BuildCreateConfigPlan(id, req, s.validation)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if err := s.repo.CreateConfig(ctx, plan.Config, s.now()); err != nil {
		return ConfigMutationResult{}, fmt.Errorf("%w: %w", ErrConfigCreateFailed, err)
	}
	return ConfigMutationResult{
		Config:                           plan.Config,
		EnqueueEvaluation:                plan.EnqueueEvaluation,
		EnqueueUnknownCountryObservation: plan.EnqueueUnknownCountryObservation,
	}, nil
}

func (s ConfigService) Update(ctx context.Context, profileID string, req PatchRequest) (ConfigMutationResult, error) {
	if s.repo == nil {
		return ConfigMutationResult{}, ErrConfigServiceMissingDependency
	}
	original, found, err := s.repo.LoadConfig(ctx, profileID)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if !found {
		return ConfigMutationResult{}, ErrProfileNotFound
	}
	original.ApplyDefaults()
	plan, err := BuildUpdateConfigPlan(original, req, s.validation)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	options := ConfigUpdateOptions{
		EvaluationChanged: plan.EvaluationChanged,
		ResetCurrentPath:  plan.ResetCurrentPath,
	}
	var deletedFingerprints []string
	if plan.ReleaseRetainedNodes {
		if s.updateWithRelease == nil {
			return ConfigMutationResult{}, ErrConfigServiceMissingDependency
		}
		deletedFingerprints, err = s.updateWithRelease(ctx, profileID, plan.Config, options)
		if err != nil {
			return ConfigMutationResult{}, fmt.Errorf("%w: %w", ErrConfigUpdateFailed, err)
		}
	} else if err := s.repo.UpdateConfig(ctx, plan.Config, options); err != nil {
		return ConfigMutationResult{}, fmt.Errorf("%w: %w", ErrConfigUpdateFailed, err)
	}
	return ConfigMutationResult{
		Config:                           plan.Config,
		DeletedFingerprints:              deletedFingerprints,
		EnqueueEvaluation:                plan.EnqueueEvaluation,
		EnqueueUnknownCountryObservation: plan.EnqueueUnknownCountryObservation,
	}, nil
}
