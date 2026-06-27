package observations

import "sync"

type ExecutableTarget struct {
	Target   NodeTarget
	Executor ProbeExecutor
}

type ObservationClock func() int64

func ExecuteBatch(repo PersistenceRepository, lookup CountryLookup, targets []ExecutableTarget, concurrency int, now ObservationClock) []RunResult {
	if len(targets) == 0 {
		return []RunResult{}
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	results := make(chan RunResult, len(targets))
	jobs := make(chan ExecutableTarget)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				results <- ExecuteNodeObservation(repo, lookup, target.Executor, target.Target, observedAt(now))
			}
		}()
	}
	for _, target := range targets {
		jobs <- target
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := make([]RunResult, 0, len(targets))
	for result := range results {
		out = append(out, result)
	}
	return out
}

func observedAt(now ObservationClock) int64 {
	if now == nil {
		return 0
	}
	return now()
}
