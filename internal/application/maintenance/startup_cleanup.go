package maintenance

import "context"

type StartupCleanupResult struct {
	CancelledCount int
	RepairedCount  int
	InvalidCount   int
	EvaluationRefs []ProfileEvaluationRef
}

type StartupCleanupService struct {
	Runs *Service
}

func (s StartupCleanupService) Execute(ctx context.Context) (StartupCleanupResult, error) {
	activeRuns, err := s.Runs.ListActive(ctx)
	if err != nil {
		return StartupCleanupResult{}, err
	}
	for _, run := range activeRuns {
		if err := s.Runs.Finish(ctx, FinishCommand{
			ID:            run.ID,
			Result:        ResultCancelled,
			ReasonCode:    ReasonExpiredAfterRestart,
			FinishedCount: run.FinishedCount,
			Detail:        RunDetail(run),
			LastError:     run.LastError,
		}); err != nil {
			return StartupCleanupResult{}, err
		}
	}
	repair, err := s.Runs.RepairDanglingProfilePaths(ctx)
	if err != nil {
		return StartupCleanupResult{}, err
	}
	detail := map[string]any{
		"cancelled_count":        len(activeRuns),
		"repaired_profile_count": repair.RepairedCount,
		"invalid_profile_count":  repair.InvalidCount,
	}
	startup, err := s.Runs.Create(ctx, CreateCommand{
		RunType:       RunTypeStartupCleanup,
		TriggerSource: TriggerStartup,
		TotalCount:    len(activeRuns),
		Detail:        detail,
	})
	if err != nil {
		return StartupCleanupResult{}, err
	}
	if err := s.Runs.Finish(ctx, FinishCommand{
		ID:            startup.ID,
		Result:        ResultSuccess,
		ReasonCode:    ReasonCompleted,
		FinishedCount: len(activeRuns),
		Detail:        RunDetail(startup),
	}); err != nil {
		return StartupCleanupResult{}, err
	}
	return StartupCleanupResult{
		CancelledCount: len(activeRuns),
		RepairedCount:  repair.RepairedCount,
		InvalidCount:   repair.InvalidCount,
		EvaluationRefs: repair.EvaluationRefs,
	}, nil
}
