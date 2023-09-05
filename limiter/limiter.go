// package limiter provides a helper to limit simultaneous requests.
package limiter

import (
	"math/rand"
	"sync/atomic"
)

// ActiveLimiter limits the number of simultaneously active requests.
// Below target it allows all requests.
// Above 2*target it rejects all requests.
// In between it limits probabilistically.
// The methods on this are thread safe.
type ActiveLimiter struct {
	target int32
	active atomic.Int32
}

// NewActiveLimiter creates a new ActiveLimiter.
func NewActiveLimiter(target int32) *ActiveLimiter {
	return &ActiveLimiter{target: target}
}

// Add adds one request to the active requests.
// Returns true iff successful.
// Must call Finish() iff returned true.
func (a *ActiveLimiter) Add() bool {
	cnt := a.active.Add(1)
	if cnt <= a.target {
		return true
	}
	fullness := float64(cnt-a.target) / float64(a.target)
	if rand.Float64() < fullness {
		a.active.Add(-1)
		return false
	}
	return true
}

// Finish marks a previously added request as finished.
func (a *ActiveLimiter) Finish() {
	a.active.Add(-1)
}
