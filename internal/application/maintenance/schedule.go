package maintenance

func DueProfileEvaluationTargets(targets []ProfileEvaluationScheduleTarget, nowMillis int64, profileIntervalSeconds, chainIntervalSeconds int) []ProfileEvaluationScheduleTarget {
	due := make([]ProfileEvaluationScheduleTarget, 0, len(targets))
	for _, target := range targets {
		if !target.AutoEvaluationEnabled {
			continue
		}
		interval := profileIntervalSeconds
		if target.ProfileType == "chain" {
			interval = chainIntervalSeconds
		}
		if target.AutoEvaluationIntervalSeconds > 0 {
			interval = target.AutoEvaluationIntervalSeconds
		}
		if interval <= 0 {
			continue
		}
		if target.LastEvaluatedAt == 0 || nowMillis-target.LastEvaluatedAt >= secondsToMillis(int64(interval)) {
			due = append(due, target)
		}
	}
	return due
}

func DueSubscriptionRefreshTargets(targets []SubscriptionRefreshScheduleTarget, nowMillis int64, defaultIntervalSeconds int) []SubscriptionRefreshScheduleTarget {
	due := make([]SubscriptionRefreshScheduleTarget, 0, len(targets))
	for _, target := range targets {
		if !target.AutoRefreshEnabled {
			continue
		}
		interval := defaultIntervalSeconds
		if target.AutoRefreshIntervalSeconds > 0 {
			interval = target.AutoRefreshIntervalSeconds
		}
		if interval > 0 && (target.UpdatedAt == 0 || nowMillis-target.UpdatedAt >= secondsToMillis(int64(interval))) {
			due = append(due, target)
		}
	}
	return due
}

func secondsToMillis(seconds int64) int64 {
	return seconds * 1000
}
