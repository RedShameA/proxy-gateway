package profile

import "errors"

var (
	ErrCandidateTimingNonNegative    = errors.New("candidate timing must be non-negative")
	ErrEvaluationIntervalNonNegative = errors.New("evaluation interval must be non-negative")
	ErrSwitchingToleranceNonNegative = errors.New("switching tolerance must be non-negative")
)

func ValidateEvaluationTiming(candidateLimit, minEvaluationIntervalSeconds, autoEvaluationIntervalSeconds int) error {
	if candidateLimit < 0 || minEvaluationIntervalSeconds < 0 {
		return ErrCandidateTimingNonNegative
	}
	if autoEvaluationIntervalSeconds < 0 {
		return ErrEvaluationIntervalNonNegative
	}
	return nil
}

func ValidateSwitchingTolerance(relativeImprovementThreshold float64, absoluteLatencyImprovementMS int) error {
	if relativeImprovementThreshold < 0 || absoluteLatencyImprovementMS < 0 {
		return ErrSwitchingToleranceNonNegative
	}
	return nil
}
