package observations

type ProbePayload struct {
	Raw       []byte
	LatencyMS int64
}

type ProbeExecutor interface {
	Probe() (ProbePayload, error)
}

func ExecuteNodeObservation(repo PersistenceRepository, lookup CountryLookup, executor ProbeExecutor, target NodeTarget, observedAt int64) RunResult {
	payload, err := executor.Probe()
	if err != nil {
		_ = PersistFailure(repo, target.ID, err.Error(), observedAt)
		return RunResult{
			NodeID: target.ID,
			Name:   target.Name,
			Error:  err.Error(),
		}
	}
	if err := PersistSuccess(repo, lookup, target.ID, payload.Raw, payload.LatencyMS, observedAt); err != nil {
		return RunResult{
			NodeID: target.ID,
			Name:   target.Name,
			Error:  err.Error(),
		}
	}
	return RunResult{
		NodeID: target.ID,
		Name:   target.Name,
		OK:     true,
	}
}
