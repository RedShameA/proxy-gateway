package evaluations

import "sync"

func RunConcurrentProbes[T any, R any](items []T, concurrency int, probe func(T) R) []R {
	if len(items) == 0 {
		return nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(items) {
		concurrency = len(items)
	}
	jobs := make(chan T)
	results := make(chan R, len(items))
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				results <- probe(item)
			}
		}()
	}
	go func() {
		for _, item := range items {
			jobs <- item
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	collected := make([]R, 0, len(items))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}
