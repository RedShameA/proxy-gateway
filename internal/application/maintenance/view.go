package maintenance

func RunToMap(run Run) map[string]any {
	detail := run.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	return map[string]any{
		"id":             run.ID,
		"run_type":       run.RunType,
		"trigger_source": run.TriggerSource,
		"target_id":      run.TargetID,
		"target_label":   run.TargetLabel,
		"state":          run.State,
		"result":         run.Result,
		"reason_code":    run.ReasonCode,
		"total_count":    run.TotalCount,
		"finished_count": run.FinishedCount,
		"detail":         detail,
		"last_error":     run.LastError,
		"created_at":     run.CreatedAt,
		"started_at":     run.StartedAt,
		"finished_at":    run.FinishedAt,
		"updated_at":     run.UpdatedAt,
	}
}
