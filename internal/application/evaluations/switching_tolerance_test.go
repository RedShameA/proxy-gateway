package evaluations

import "testing"

func TestSelectFastestPathUsesConfiguredSwitchingTolerance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		candidateDuration int64
		currentDuration   int64
		relativeThreshold float64
		absoluteThreshold int
		wantSwitch        bool
	}{
		{
			name:              "uses absolute threshold when relative threshold is higher",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 100,
			wantSwitch:        true,
		},
		{
			name:              "uses relative threshold when absolute threshold is higher",
			candidateDuration: 790,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 300,
			wantSwitch:        true,
		},
		{
			name:              "does not switch below configured thresholds",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 200,
			wantSwitch:        false,
		},
		{
			name:              "zero thresholds allow any latency improvement",
			candidateDuration: 999,
			currentDuration:   1000,
			relativeThreshold: 0,
			absoluteThreshold: 0,
			wantSwitch:        true,
		},
		{
			name:              "zero relative threshold disables relative condition",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0,
			absoluteThreshold: 200,
			wantSwitch:        false,
		},
		{
			name:              "zero absolute threshold disables absolute condition",
			candidateDuration: 850,
			currentDuration:   1000,
			relativeThreshold: 0.2,
			absoluteThreshold: 0,
			wantSwitch:        false,
		},
		{
			name:              "never switches to equal latency even when thresholds are zero",
			candidateDuration: 1000,
			currentDuration:   1000,
			relativeThreshold: 0,
			absoluteThreshold: 0,
			wantSwitch:        false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			selection := SelectFastestPath(FastestSelectionInput{
				CurrentNodeID:                "node-current",
				CurrentDurationMS:            tt.currentDuration,
				BestNodeID:                   "node-candidate",
				BestDurationMS:               tt.candidateDuration,
				RelativeImprovementThreshold: tt.relativeThreshold,
				AbsoluteLatencyImprovementMS: tt.absoluteThreshold,
			})
			gotSwitch := selection.SelectedNodeID == "node-candidate"
			if gotSwitch != tt.wantSwitch {
				t.Fatalf("switch = %v, want %v; selection = %#v", gotSwitch, tt.wantSwitch, selection)
			}
		})
	}
}
