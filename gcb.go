package gcb

import (
	"net/http"
	"time"
)

type (
	// tripper
	tripper struct {
		http.RoundTripper
	}

	// Option represents an option for retry.
	Option func(*Config)

	Config struct {
		maxRetries    uint32
		maxRequests   uint32

		interval time.Duration
		timeout time.Duration
		maxWait time.Duration
		minWait time.Duration

		readyToTrip   ReadyToTrip
		onStateChange OnStateChange
	}
)

func NewRoundTripper(opts ...Option) *tripper {
	cb := newCircuitBreaker(opts...)
	t := &tripper{
		RoundTripper: cb,
	}
	return t
}

// state
func (t *tripper) state() State {
	return t.RoundTripper.(*circuit).GetState()
}

// WithMaxRetries sets the maximum maxRetries according
// to the retry policy
func WithMaxRetries(maxRetries uint32) Option {
	return func(config *Config) {
		config.maxRetries = maxRetries
	}
}

func WithReadyToTrip(fn ReadyToTrip) Option {
	return func(config *Config) {
		config.readyToTrip = fn
	}
}
