package maintenance

import (
	"context"
	"errors"
	"strings"
)

const (
	StateQueued   = "queued"
	StateRunning  = "running"
	StateFinished = "finished"

	ResultSuccess = "success"

	ReasonCompleted = "completed"

	NodeObservationScopeAllNodes = "all_nodes"
	NodeObservationScopeDueNodes = "due_nodes"
)

var ErrRunNotFound = errors.New("maintenance run not found")

type Run struct {
	ID            string
	RunType       string
	TriggerSource string
	TargetID      string
	TargetLabel   string
	State         string
	Result        string
	ReasonCode    string
	TotalCount    int
	FinishedCount int
	Detail        map[string]any
	LastError     string
	CreatedAt     int64
	StartedAt     int64
	FinishedAt    int64
	UpdatedAt     int64
}

type CreateCommand struct {
	RunType       string
	TriggerSource string
	TargetID      string
	TargetLabel   string
	TotalCount    int
	Detail        map[string]any
}

type FinishCommand struct {
	ID            string
	Result        string
	ReasonCode    string
	FinishedCount int
	Detail        map[string]any
	LastError     string
}

type FinishUpdate struct {
	ID            string
	Result        string
	ReasonCode    string
	FinishedCount int
	Detail        map[string]any
	LastError     string
	NowMillis     int64
}

type ListFilter struct {
	RunType  string
	TargetID string
	State    string
	Result   string
	Page     int
	PageSize int
}

type ListResult struct {
	Items    []Run
	Total    int
	Page     int
	PageSize int
}

type DanglingProfileRepairResult struct {
	RepairedCount  int
	InvalidCount   int
	EvaluationRefs []ProfileEvaluationRef
}

type ProfileEvaluationRef struct {
	ID   string
	Name string
}

type Repository interface {
	Insert(ctx context.Context, run Run) error
	Load(ctx context.Context, id string) (Run, error)
	Start(ctx context.Context, id string, nowMillis int64) error
	SetTotal(ctx context.Context, id string, totalCount int, nowMillis int64) error
	Finish(ctx context.Context, update FinishUpdate) error
	ClaimNext(ctx context.Context, runType string, nowMillis int64) (Run, bool, error)
	List(ctx context.Context, filter ListFilter) (ListResult, error)
	ListProfileEvents(ctx context.Context, profileID string, limit int) ([]Run, error)
	ListUnfinished(ctx context.Context, runType string) ([]Run, error)
	ListActive(ctx context.Context) ([]Run, error)
	RepairDanglingProfilePaths(ctx context.Context, nowMillis int64) (DanglingProfileRepairResult, error)
}

type IDGenerator func(prefix string) (string, error)

type Clock func() int64

type Service struct {
	repo Repository
	ids  IDGenerator
	now  Clock
}

func NewService(repo Repository, ids IDGenerator, now Clock) *Service {
	return &Service{repo: repo, ids: ids, now: now}
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Run, error) {
	totalCount := cmd.TotalCount
	if totalCount < 0 {
		totalCount = 0
	}
	id, err := s.ids("run")
	if err != nil {
		return Run{}, err
	}
	now := s.now()
	run := Run{
		ID:            id,
		RunType:       cmd.RunType,
		TriggerSource: cmd.TriggerSource,
		TargetID:      cmd.TargetID,
		TargetLabel:   cmd.TargetLabel,
		State:         StateQueued,
		TotalCount:    totalCount,
		Detail:        copyDetail(cmd.Detail),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.Insert(ctx, run); err != nil {
		return Run{}, err
	}
	return s.repo.Load(ctx, id)
}

func (s *Service) Load(ctx context.Context, id string) (Run, error) {
	return s.repo.Load(ctx, id)
}

func (s *Service) Start(ctx context.Context, id string) error {
	return s.repo.Start(ctx, id, s.now())
}

func (s *Service) SetTotal(ctx context.Context, id string, totalCount int) error {
	if totalCount < 0 {
		totalCount = 0
	}
	return s.repo.SetTotal(ctx, id, totalCount, s.now())
}

func (s *Service) Finish(ctx context.Context, cmd FinishCommand) error {
	result := cmd.Result
	if result == "" {
		result = ResultSuccess
	}
	reasonCode := cmd.ReasonCode
	if reasonCode == "" {
		reasonCode = ReasonCompleted
	}
	finishedCount := cmd.FinishedCount
	if finishedCount < 0 {
		finishedCount = 0
	}
	return s.repo.Finish(ctx, FinishUpdate{
		ID:            cmd.ID,
		Result:        result,
		ReasonCode:    reasonCode,
		FinishedCount: finishedCount,
		Detail:        copyDetail(cmd.Detail),
		LastError:     strings.TrimSpace(cmd.LastError),
		NowMillis:     s.now(),
	})
}

func (s *Service) ClaimNext(ctx context.Context, runType string) (Run, bool, error) {
	return s.repo.ClaimNext(ctx, runType, s.now())
}

func (s *Service) ClaimQueuedRunsOfType(ctx context.Context, runType string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 1
	}
	runs := make([]Run, 0, limit)
	for i := 0; i < limit; i++ {
		run, ok, err := s.ClaimNext(ctx, runType)
		if err != nil {
			return runs, err
		}
		if !ok {
			break
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (s *Service) List(ctx context.Context, filter ListFilter) (ListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 10
	}
	return s.repo.List(ctx, filter)
}

func (s *Service) ListProfileEvents(ctx context.Context, profileID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.ListProfileEvents(ctx, strings.TrimSpace(profileID), limit)
}

func (s *Service) ListUnfinished(ctx context.Context, runType string) ([]Run, error) {
	return s.repo.ListUnfinished(ctx, strings.TrimSpace(runType))
}

func (s *Service) ListActive(ctx context.Context) ([]Run, error) {
	return s.repo.ListActive(ctx)
}

func (s *Service) CancelUnfinishedNodeObservationAggregateRuns(ctx context.Context, reasonCode string) error {
	runs, err := s.ListUnfinished(ctx, RunTypeNodeObservation)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if !IsNodeObservationAggregateRun(run) {
			continue
		}
		if err := s.Finish(ctx, FinishCommand{
			ID:            run.ID,
			Result:        ResultCancelled,
			ReasonCode:    reasonCode,
			FinishedCount: run.FinishedCount,
			Detail:        RunDetail(run),
			LastError:     run.LastError,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) HasUnfinishedNodeObservationAggregateRun(ctx context.Context) (bool, error) {
	runs, err := s.ListUnfinished(ctx, RunTypeNodeObservation)
	if err != nil {
		return false, err
	}
	for _, run := range runs {
		if IsNodeObservationAggregateRun(run) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) RepairDanglingProfilePaths(ctx context.Context) (DanglingProfileRepairResult, error) {
	return s.repo.RepairDanglingProfilePaths(ctx, s.now())
}

func IsNodeObservationAggregateRun(run Run) bool {
	scope, _ := RunDetail(run)["target_scope"].(string)
	return scope == NodeObservationScopeAllNodes || scope == NodeObservationScopeDueNodes
}

func copyDetail(detail map[string]any) map[string]any {
	if detail == nil {
		return map[string]any{}
	}
	copied := make(map[string]any, len(detail))
	for key, value := range detail {
		copied[key] = value
	}
	return copied
}
