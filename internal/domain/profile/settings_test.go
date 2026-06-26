package profile

import (
	"errors"
	"testing"
)

func TestValidateEvaluationTiming(t *testing.T) {
	if err := ValidateEvaluationTiming(0, 0, 0); err != nil {
		t.Fatalf("valid timing = %v", err)
	}
	if err := ValidateEvaluationTiming(-1, 0, 0); !errors.Is(err, ErrCandidateTimingNonNegative) {
		t.Fatalf("candidate limit error = %v, want ErrCandidateTimingNonNegative", err)
	}
	if err := ValidateEvaluationTiming(0, -1, 0); !errors.Is(err, ErrCandidateTimingNonNegative) {
		t.Fatalf("min interval error = %v, want ErrCandidateTimingNonNegative", err)
	}
	if err := ValidateEvaluationTiming(0, 0, -1); !errors.Is(err, ErrEvaluationIntervalNonNegative) {
		t.Fatalf("auto interval error = %v, want ErrEvaluationIntervalNonNegative", err)
	}
}

func TestValidateSwitchingTolerance(t *testing.T) {
	if err := ValidateSwitchingTolerance(0.2, 100); err != nil {
		t.Fatalf("valid tolerance = %v", err)
	}
	if err := ValidateSwitchingTolerance(-0.1, 100); !errors.Is(err, ErrSwitchingToleranceNonNegative) {
		t.Fatalf("relative error = %v, want ErrSwitchingToleranceNonNegative", err)
	}
	if err := ValidateSwitchingTolerance(0.2, -1); !errors.Is(err, ErrSwitchingToleranceNonNegative) {
		t.Fatalf("absolute error = %v, want ErrSwitchingToleranceNonNegative", err)
	}
}
