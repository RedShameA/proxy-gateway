package nodes

type ObservationSnapshot struct {
	Found         bool
	Unavailable   bool
	Usable        bool
	EgressIP      string
	EgressCountry string
	LatencyMS     int64
	LastError     string
	LastSuccessAt int64
	LastFailureAt int64
}

func ObservationSnapshotFromRecord(record ObservationRecord, found bool) ObservationSnapshot {
	if !found {
		return ObservationSnapshot{}
	}
	return ObservationSnapshot{
		Found:         true,
		Usable:        record.Usable,
		EgressIP:      record.EgressIP,
		EgressCountry: record.EgressCountry,
		LatencyMS:     record.LatencyMS,
		LastError:     record.LastError,
		LastSuccessAt: record.LastSuccessAt,
		LastFailureAt: record.LastFailureAt,
	}
}

func UnavailableObservationSnapshot() ObservationSnapshot {
	return ObservationSnapshot{Found: true, Unavailable: true}
}

func (snapshot ObservationSnapshot) Map() map[string]any {
	if !snapshot.Found {
		return nil
	}
	if snapshot.Unavailable {
		return map[string]any{"usable": false}
	}
	return map[string]any{
		"usable":          snapshot.Usable,
		"egress_ip":       snapshot.EgressIP,
		"egress_country":  snapshot.EgressCountry,
		"latency_ms":      snapshot.LatencyMS,
		"last_error":      snapshot.LastError,
		"last_success_at": snapshot.LastSuccessAt,
		"last_failure_at": snapshot.LastFailureAt,
		"stale":           !snapshot.Usable && snapshot.LastSuccessAt > 0 && snapshot.LastFailureAt >= snapshot.LastSuccessAt,
	}
}

func BuildDetail(record Record, sources []SourceRecord, observation ObservationSnapshot) map[string]any {
	return map[string]any{
		"id":          record.ID,
		"name":        record.Name,
		"type":        record.Type,
		"protocol":    record.Type,
		"server":      record.Server,
		"server_port": record.ServerPort,
		"username":    record.Username,
		"password":    record.Password,
		"enabled":     record.Enabled,
		"sources":     SourceViews(sources),
		"observation": observation.Map(),
		"raw_json":    record.RawJSON,
	}
}

func BuildListItem(record Record, sources []SourceRecord, observation ObservationSnapshot) map[string]any {
	var egressIP any
	if observation.EgressIP != "" {
		egressIP = observation.EgressIP
	}
	var latencyMS any
	if observation.Usable && observation.LatencyMS > 0 {
		latencyMS = observation.LatencyMS
	}
	var lastObservedAt any
	observedAt := observation.LastSuccessAt
	if observation.LastFailureAt > observedAt {
		observedAt = observation.LastFailureAt
	}
	if observedAt > 0 {
		lastObservedAt = observedAt
	}
	return map[string]any{
		"id":                     record.ID,
		"name":                   record.Name,
		"type":                   record.Type,
		"protocol":               record.Type,
		"server":                 record.Server,
		"server_port":            record.ServerPort,
		"username":               record.Username,
		"password":               record.Password,
		"enabled":                record.Enabled,
		"state":                  NodeState(record.Enabled, observation),
		"sources":                SourceViews(sources),
		"observation":            observation.Map(),
		"egress_ip":              egressIP,
		"egress_country":         observation.EgressCountry,
		"observation_latency_ms": latencyMS,
		"last_observed_at":       lastObservedAt,
		"last_error":             observation.LastError,
	}
}

func NodeState(enabled bool, observation ObservationSnapshot) string {
	if !enabled {
		return StateDisabled
	}
	if !observation.Found {
		return StatePendingObservation
	}
	if observation.Usable {
		return StateUsable
	}
	return StateUnusable
}

func SourceViews(sources []SourceRecord) []map[string]any {
	out := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		out = append(out, map[string]any{
			"source_id":    source.SourceID,
			"source_name":  source.SourceName,
			"source_type":  source.SourceType,
			"display_name": source.DisplayName,
		})
	}
	return out
}
