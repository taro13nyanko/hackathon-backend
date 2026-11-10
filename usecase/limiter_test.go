package usecase

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	l := &Limiter{Max: 2, Window: time.Minute, Now: func() time.Time { return now }}
	if !l.Allow("a") || !l.Allow("a") || l.Allow("a") {
		t.Fatal("1分に2回まで")
	}
	if !l.Allow("b") {
		t.Fatal("人ごとに数える")
	}
	now = now.Add(time.Minute)
	if !l.Allow("a") {
		t.Fatal("1分たてばまた使える")
	}
	var none *Limiter
	for i := 0; i < 100; i++ {
		if !none.Allow("a") {
			t.Fatal("nil は制限しない")
		}
	}
}
