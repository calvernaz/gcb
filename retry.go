package gcb

import (
	"context"
	"errors"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

var (
	errMaxRetriesReached = errors.New("exceeded retry limit")

	// Default retry configuration
	defaultRetryWaitMin = 1 * time.Second
	defaultRetryWaitMax = 30 * time.Second
	defaultRetryMax     = 4
)

type (
	// CheckRetry specifies a policy for handling shouldRetry. It is called
	// following each request with the response and error values returned by
	// the http.Client. If CheckRetry returns false, the Client stops retrying
	// and returns the response to the caller. If CheckRetry returns an error,
	// that error value is returned in lieu of the error from the request. The
	// Client will close any response body when retrying, but if the retry is
	// aborted it is up to the CheckRetry callback to properly close any
	// response body before returning.
	CheckRetry func(ctx context.Context, resp *http.Response, err error) (bool, error)

	// Function signature of retryable function
	DoFunc func() (*http.Response, error)

	// Retrier
	Retrier struct {
		config *Config

		// Backoff specifies the policy for how long to wait between shouldRetry
		Backoff Backoff

		RetryWaitMin time.Duration // Minimum time to wait
		RetryWaitMax time.Duration // Maximum time to wait
		RetryMax     int           // Maximum number of retries

		// CheckRetry specifies the policy for handling reties, and is called
		// after each request. The default policy is DefaultRetryPolicy.
		CheckRetry CheckRetry

		// Limiter specifies the policy that controls the request rate.
		Limiter *rate.Limiter
	}
)

func NewRetrier(opts ...Option) *Retrier {
	//default
	config := &Config{
		delay:         100 * time.Millisecond,
		lastErrorOnly: false,
		retries: defaultRetryMax,
	}

	// apply opts
	for _, opt := range opts {
		opt(config)
	}

	return &Retrier{
		config:     config,
		RetryMax: config.retries,
		CheckRetry: DefaultRetryPolicy,
		Backoff:    DefaultBackoff,
		Limiter:    rate.NewLimiter(rate.Every(5 * time.Millisecond), 200),
	}
}

func (r *Retrier) retryPolicy(ctx context.Context, res *http.Response, err error) (bool, error) {
	// rate limiter allowance
	if !r.Limiter.Allow() {
		return false, rateLimitExceeded
	}
	return r.CheckRetry(ctx, res, err)
}

// DefaultRetryPolicy provides a default callback for Client.CheckRetry, which
// will retry on connection errors and server errors.
func DefaultRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// do not retry on context.Canceled or context.DeadlineExceeded
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if err != nil {
		return true, err
	}
	// Check the response code. We retry on 500-range responses to allow
	// the server time to recover, as 500's are typically not permanent
	// errors and may relate to outages on the server side. This will catch
	// invalid response codes as well, like 0 and 999.
	if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != 501) {
		return true, nil
	}

	return false, nil
}
