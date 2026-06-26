package app

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	adminSessionTTL       = 30 * 24 * time.Hour
	adminLoginWindow      = 15 * time.Minute
	adminLoginLockout     = time.Minute
	adminLoginMaxFailures = 5
)

type adminLoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]adminLoginAttempt
}

type adminLoginAttempt struct {
	Failures       int
	FirstFailureAt int64
	LockedUntil    int64
}

func newAdminLoginLimiter() *adminLoginLimiter {
	return &adminLoginLimiter{attempts: map[string]adminLoginAttempt{}}
}

func (l *adminLoginLimiter) allow(key string, now int64) (int, bool) {
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
	if attempt.FirstFailureAt > 0 && now-attempt.FirstFailureAt > int64(adminLoginWindow/time.Millisecond) {
		delete(l.attempts, key)
	}
	return 0, true
}

func (l *adminLoginLimiter) recordFailure(key string, now int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.attempts[key]
	if attempt.FirstFailureAt == 0 || now-attempt.FirstFailureAt > int64(adminLoginWindow/time.Millisecond) {
		attempt = adminLoginAttempt{FirstFailureAt: now}
	}
	attempt.Failures++
	if attempt.Failures >= adminLoginMaxFailures {
		attempt.LockedUntil = now + int64(adminLoginLockout/time.Millisecond)
	}
	l.attempts[key] = attempt
}

func (l *adminLoginLimiter) recordSuccess(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func adminLoginLimitKey(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
