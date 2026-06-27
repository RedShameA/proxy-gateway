package admin

import (
	"sync"
	"time"
)

const (
	LoginWindow      = 15 * time.Minute
	LoginLockout     = time.Minute
	LoginMaxFailures = 5
)

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]LoginAttempt
}

type LoginAttempt struct {
	Failures       int
	FirstFailureAt int64
	LockedUntil    int64
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{attempts: map[string]LoginAttempt{}}
}

func (l *LoginLimiter) Allow(key string, now int64) (int, bool) {
	if l == nil {
		return 0, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.attempts[key]
	if attempt.LockedUntil > now {
		retryAfter := int((attempt.LockedUntil - now + 999) / 1000)
		if retryAfter < 1 {
			retryAfter = 1
		}
		return retryAfter, false
	}
	if attempt.FirstFailureAt > 0 && now-attempt.FirstFailureAt > int64(LoginWindow/time.Millisecond) {
		delete(l.attempts, key)
	}
	return 0, true
}

func (l *LoginLimiter) RecordFailure(key string, now int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.attempts[key]
	if attempt.FirstFailureAt == 0 || now-attempt.FirstFailureAt > int64(LoginWindow/time.Millisecond) {
		attempt = LoginAttempt{FirstFailureAt: now}
	}
	attempt.Failures++
	if attempt.Failures >= LoginMaxFailures {
		attempt.LockedUntil = now + int64(LoginLockout/time.Millisecond)
	}
	l.attempts[key] = attempt
}

func (l *LoginLimiter) RecordSuccess(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
