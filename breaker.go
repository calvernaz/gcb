package gcb

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

var (
	// ErrTooManyRequests is returned when the CB state is half open and the requests count is over the cb maxRequests
	ErrTooManyRequests = errors.New("too many requests")
	// ErrOpenState is returned when the CB state is open
	ErrOpenState = errors.New("circuit breaker is open")
)

type (
	State int8

	// Counts holds the numbers of requests and their successes/failures.
	// Breaker clears the internal Counts either
	// on the change of the state or at the closed-state intervals.
	// Counts ignores the results of the requests sent before clearing.
	Counts struct {
		Requests             uint32
		TotalSuccesses       uint32
		TotalFailures        uint32
		ConsecutiveSuccesses uint32
		ConsecutiveFailures  uint32
	}

	ReadyToTrip func(counts Counts) bool

	OnStateChange func(name string, from State, to State)

	// Breaker is a state machine to prevent sending requests that are likely to fail.
	Breaker struct {
		// Name is the name of the CircuitBreaker.
		name          string
		// MaxRequests is the maximum number of requests allowed to pass through
		// when the CircuitBreaker is half-open.
		// If MaxRequests is 0, the CircuitBreaker allows only 1 request.
		maxRequests   uint32
		// Interval is the cyclic period of the closed state
		// for the CircuitBreaker to clear the internal Counts.
		// If Interval is 0, the CircuitBreaker doesn't clear internal Counts during the closed state.
		interval      time.Duration
		// Timeout is the period of the open state,
		// after which the state of the CircuitBreaker becomes half-open.
		// If Timeout is 0, the timeout value of the CircuitBreaker is set to 60 seconds.
		timeout       time.Duration
		// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
		// If ReadyToTrip returns true, the CircuitBreaker will be placed into the open state.
		// If ReadyToTrip is nil, default ReadyToTrip is used.
		// Default ReadyToTrip returns true when the number of consecutive failures is more than 5.
		readyToTrip   func(counts Counts) bool
		// OnStateChange is called whenever the state of the CircuitBreaker changes.
		onStateChange func(name string, from State, to State)

		mutex      sync.Mutex
		state      State
		generation uint64
		counts     Counts
		expiry     time.Time
	}
)

const (
	defaultTimeout = time.Duration(60) * time.Second
	defaultInterval = time.Duration(30) * time.Second
	defaultMaxRequests = 1

	Close State = iota
	HalfOpen
	Open
)

func NewBreaker(opts ...Option) *Breaker {
	// defaults
	config := &Config{
		timeout: defaultTimeout,
		interval:  defaultInterval,
		maxRequests: defaultMaxRequests,
		readyToTrip: defaultReadyToTrip,
		onStateChange:  defaultOnStateChange,
	}

	// apply opts
	for _, opt := range opts {
		opt(config)
	}

	cb := &Breaker{
		timeout: config.timeout,
		maxRequests: config.maxRequests,

		readyToTrip: config.readyToTrip,
		onStateChange: config.onStateChange,

		state: Close,
	}

	cb.toNewGeneration(time.Now())
	return cb
}

// TODO: why 3?
func defaultReadyToTrip(counts Counts) bool {
	return counts.ConsecutiveFailures > 3
}

func defaultOnStateChange(name string, from State, to State) {
	// noop
}

func (s State) String() string {
	switch s {
	case Open:
		return "Open"
	case HalfOpen:
		return "HalfOpen"
	case Close:
		return "Close"
	}
	return ""
}

func (c *Counts) onRequest() {
	c.Requests++
}

func (c *Counts) onSuccess() {
	c.TotalSuccesses++
	c.ConsecutiveSuccesses++
	c.ConsecutiveFailures = 0
}

func (c *Counts) onFailure() {
	c.TotalFailures++
	c.ConsecutiveFailures++
	c.ConsecutiveSuccesses = 0
}

func (c *Counts) clear() {
	c.Requests = 0
	c.TotalSuccesses = 0
	c.TotalFailures = 0
	c.ConsecutiveSuccesses = 0
	c.ConsecutiveFailures = 0
}


// Execute runs the given request if the CircuitBreaker accepts it.
// Execute returns an error instantly if the CircuitBreaker rejects the request.
// Otherwise, Execute returns the result of the request.
// If a panic occurs in the request, the CircuitBreaker handles it as an error
// and causes the same panic again.
func (cb *Breaker) Execute(req func() (*http.Response, error)) (*http.Response, error) {
	generation, err := cb.beforeRequest()
	if err != nil {
		return nil, err
	}

	defer func() {
		e := recover()
		if e != nil {
			cb.afterRequest(generation, false)
			panic(e)
		}
	}()

	result, err := req()
	cb.afterRequest(generation, err == nil)
	return result, err
}

func (cb *Breaker) beforeRequest() (uint64, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	state, generation := cb.currentState(time.Now())

	if state == Open {
		return generation, ErrOpenState
	} else if state == HalfOpen && cb.counts.Requests >= cb.maxRequests {
		return generation, ErrTooManyRequests
	}

	cb.counts.onRequest()
	return generation, nil
}

func (cb *Breaker) afterRequest(before uint64, success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

func (cb *Breaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts.clear()

	var zero time.Time
	switch cb.state {
	case Close:
		if cb.interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.interval)
		}
	case Open:
		cb.expiry = now.Add(cb.timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

func (cb *Breaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case Close:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case Open:
		if cb.expiry.Before(now) {
			cb.setState(HalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

func (cb *Breaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}
}

func (cb *Breaker) onSuccess(state State, now time.Time) {
	switch state {
	case Close:
		cb.counts.onSuccess()
	case HalfOpen:
		cb.counts.onSuccess()
		if cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
			cb.setState(Close, now)
		}
	}
}

func (cb *Breaker) onFailure(state State, now time.Time) {
	switch state {
	case Close:
		cb.counts.onFailure()
		if cb.readyToTrip(cb.counts) {
			cb.setState(Open, now)
		}
	case HalfOpen:
		cb.setState(Open, now)
	}
}
