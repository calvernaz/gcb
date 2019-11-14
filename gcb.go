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
		delay         time.Duration
		lastErrorOnly bool
		retries       int
	}
)

func NewRoundTripper(opts ...Option) *tripper {
	circuit := newCircuit(opts...)
	t := &tripper{
		RoundTripper: circuit,
	}
	return t
}

// WithMaxRetries sets the maximum retries according
// to the retry policy
func WithMaxRetries(maxRetries int) Option {
	return func(config *Config) {
		config.retries = maxRetries
	}
}
