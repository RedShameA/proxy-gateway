package evaluations

type CandidateProbeResult[T any] struct {
	Candidate  T
	DurationMS int64
	HTTPStatus int
	Timings    ProbeTimings
	Err        error
}

type CandidateProbeMeasurement struct {
	DurationMS int64
	HTTPStatus int
	Timings    ProbeTimings
}

type ProbeTimings struct {
	DialDurationMS int64
	HTTPDurationMS int64
	CacheWaitMS    int64
	CacheBuildMS   int64
	OutboundDialMS int64
}

func (r CandidateProbeResult[T]) OK() bool {
	return r.Err == nil
}

func (r CandidateProbeResult[T]) ErrorMessage() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return "test url probe failed without error detail"
}

type FastestCandidateProbeRun[T any] struct {
	Results []CandidateProbeResult[T]
	Summary FastestProbeSummary
}

type ChainCandidateProbeRun[T any] struct {
	Results []CandidateProbeResult[T]
	Summary ChainProbeSummary
}

func ExecuteFastestCandidateProbes[T any](candidates []T, concurrency int, currentNodeID string, nodeID func(T) string, probe func(T) (CandidateProbeMeasurement, error)) FastestCandidateProbeRun[T] {
	results := RunConcurrentProbes(
		candidates,
		concurrency,
		func(candidate T) CandidateProbeResult[T] {
			measurement, err := probe(candidate)
			return CandidateProbeResult[T]{
				Candidate:  candidate,
				DurationMS: measurement.DurationMS,
				HTTPStatus: measurement.HTTPStatus,
				Timings:    measurement.Timings,
				Err:        err,
			}
		},
	)
	probeResults := make([]FastestProbeResult, 0, len(results))
	for _, result := range results {
		probeResults = append(probeResults, FastestProbeResult{
			NodeID:     nodeID(result.Candidate),
			DurationMS: result.DurationMS,
			OK:         result.OK(),
			Error:      result.ErrorMessage(),
		})
	}
	return FastestCandidateProbeRun[T]{
		Results: results,
		Summary: SummarizeFastestProbeResults(currentNodeID, probeResults),
	}
}

func ExecuteChainCandidateProbes[T any](candidates []T, concurrency int, currentNodeID, currentExitNodeID string, frontNodeID func(T) string, exitNodeID func(T) string, probe func(T) (CandidateProbeMeasurement, error)) ChainCandidateProbeRun[T] {
	results := RunConcurrentProbes(
		candidates,
		concurrency,
		func(candidate T) CandidateProbeResult[T] {
			measurement, err := probe(candidate)
			return CandidateProbeResult[T]{
				Candidate:  candidate,
				DurationMS: measurement.DurationMS,
				HTTPStatus: measurement.HTTPStatus,
				Timings:    measurement.Timings,
				Err:        err,
			}
		},
	)
	probeResults := make([]ChainProbeResult, 0, len(results))
	for _, result := range results {
		probeResults = append(probeResults, ChainProbeResult{
			FrontNodeID: frontNodeID(result.Candidate),
			ExitNodeID:  exitNodeID(result.Candidate),
			DurationMS:  result.DurationMS,
			OK:          result.OK(),
			Error:       result.ErrorMessage(),
		})
	}
	return ChainCandidateProbeRun[T]{
		Results: results,
		Summary: SummarizeChainProbeResults(currentNodeID, currentExitNodeID, probeResults),
	}
}
