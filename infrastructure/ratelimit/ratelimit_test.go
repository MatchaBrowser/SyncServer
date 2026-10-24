package ratelimit

import (
	"testing"
	"time"
)

// clock is a manual clock for the tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(limit int, window time.Duration, c *clock) *Limiter {
	l := New(limit, window)
	l.now = c.now
	return l
}

func TestLimiter_SlidingWindow(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	l := newTestLimiter(3, 15*time.Minute, c)

	for i := range 3 {
		if ok, _ := l.Allow("ip|account"); !ok {
			t.Fatalf("attempt %d rejected", i+1)
		}
		c.advance(time.Minute)
	}
	ok, retryAfter := l.Allow("ip|account")
	if ok {
		t.Fatal("the 4th attempt within 15 minutes must be rejected")
	}
	// The first attempt (12:00) leaves the window at 12:15; it is 12:03.
	if retryAfter != 12*time.Minute {
		t.Errorf("retryAfter %v, want 12m", retryAfter)
	}

	// Another key has its own count.
	if ok, _ := l.Allow("other"); !ok {
		t.Error("another key must not be limited")
	}

	c.advance(12 * time.Minute)
	if ok, _ := l.Allow("ip|account"); !ok {
		t.Error("an attempt must be allowed once the oldest one left the window")
	}
}

func TestLimiter_RejectedAttemptsAreNotCounted(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := newTestLimiter(1, time.Minute, c)
	l.Allow("k")
	for range 10 {
		l.Allow("k") // rejected
	}
	c.advance(time.Minute + time.Second)
	if ok, _ := l.Allow("k"); !ok {
		t.Error("rejected attempts must not extend the block")
	}
}

func TestAllowAll(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	perPair := newTestLimiter(2, 15*time.Minute, c)
	perAccount := newTestLimiter(3, time.Hour, c)

	attempt := func(ip string) bool {
		ok, _ := AllowAll(Attempt{perPair, ip + "|account"}, Attempt{perAccount, "account"})
		return ok
	}
	if !attempt("ip1") || !attempt("ip1") {
		t.Fatal("the first two attempts from ip1 must pass")
	}
	if attempt("ip1") {
		t.Error("a third attempt from ip1 must be rejected by the pair limit")
	}
	// Rejected by the pair limit: not counted against the account.
	if !attempt("ip2") {
		t.Error("ip2 must pass: the account has 2 attempts out of 3")
	}
	if attempt("ip3") {
		t.Error("a fourth attempt on the account must be rejected")
	}
}

func TestLimiter_Prune(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := newTestLimiter(5, time.Minute, c)
	l.Allow("a")
	l.Allow("b")
	c.advance(2 * time.Minute)
	l.Prune()
	if l.Len() != 0 {
		t.Errorf("%d keys left after their window, want 0", l.Len())
	}
}
