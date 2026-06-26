package app

import "time"

const millisecondsPerSecond int64 = 1000

func unixMillisNow() int64 {
	return time.Now().UnixMilli()
}

func secondsToMillis(seconds int64) int64 {
	if seconds <= 0 {
		return 0
	}
	return seconds * millisecondsPerSecond
}
