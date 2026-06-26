package app

import "testing"

func TestClearlyBetterUsesConfiguredSwitchingTolerance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		candidateDuration int64
		currentDuration   int64
		relativeThreshold float64
		absoluteThreshold int
		want              bool
	}{
		{
			name:              "uses absolute threshold when relative threshold is higher",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 100,
			want:              true,
		},
		{
			name:              "uses relative threshold when absolute threshold is higher",
			candidateDuration: 790,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 300,
			want:              true,
		},
		{
			name:              "does not switch below configured thresholds",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 200,
			want:              false,
		},
		{
			name:              "zero thresholds allow any latency improvement",
			candidateDuration: 999,
			currentDuration:   1000,
			relativeThreshold: 0,
			absoluteThreshold: 0,
			want:              true,
		},
		{
			name:              "zero relative threshold disables relative condition",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0,
			absoluteThreshold: 200,
			want:              false,
		},
		{
			name:              "zero absolute threshold disables absolute condition",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 0,
			want:              false,
		},
		{
			name:              "never switches to equal latency even when thresholds are zero",
			candidateDuration: 1000,
			currentDuration:   1000,
			relativeThreshold: 0,
			absoluteThreshold: 0,
			want:              false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := clearlyBetter(tt.candidateDuration, tt.currentDuration, tt.relativeThreshold, tt.absoluteThreshold)
			if got != tt.want {
				t.Fatalf("clearlyBetter() = %v, want %v", got, tt.want)
			}
		})
	}
}
