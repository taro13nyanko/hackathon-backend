package usecase

import (
	"sync"
	"time"
)

// Limiter は「1人あたり Window の間に Max 回まで」を数える（サーバー1台のメモリ内）。
// nil の Limiter は制限しない（テストで使う）。
type Limiter struct {
	Max    int
	Window time.Duration
	Now    func() time.Time

	mu   sync.Mutex
	used map[string][]time.Time
}

// NewLimiter は Window あたり max 回の Limiter を作る。
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window}
}

// Allow は key（ふつうは会員 ID）がもう1回使ってよいかを返し、よければ1回と数える。
func (l *Limiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	if l.Now != nil {
		now = l.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.used == nil {
		l.used = map[string][]time.Time{}
	}
	// 古い記録を捨てる。記録が無くなった人は表から消す（表が増え続けないように）
	for k, ts := range l.used {
		if len(ts) > 0 && now.Sub(ts[len(ts)-1]) >= l.Window {
			delete(l.used, k)
		}
	}
	recent := l.used[key][:0]
	for _, t := range l.used[key] {
		if now.Sub(t) < l.Window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.Max {
		l.used[key] = recent
		return false
	}
	l.used[key] = append(recent, now)
	return true
}
