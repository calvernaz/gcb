package gcb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var errMaxRetriesReached = errors.New("exceeded retry limit")

var (
	// Default retry configuration
	defaultRetryWaitMin = 1 * time.Second
	defaultRetryWaitMax = 30 * time.Second
	defaultRetryMax     = 4
)

// CheckRetry specifies a policy for handling retries. It is called
// following each request with the response and error values returned by
// the http.Client. If CheckRetry returns false, the Client stops retrying
// and returns the response to the caller. If CheckRetry returns an error,
// that error value is returned in lieu of the error from the request. The
// Client will close any response body when retrying, but if the retry is
// aborted it is up to the CheckRetry callback to properly close any
// response body before returning.
type CheckRetry func(ctx context.Context, resp *http.Response, err error) (bool, error)

// Function signature of retryable function
type DoFunc func() (*http.Response, error)

// Option represents an option for retry.
type Option func(*Config)

type Config struct {
	delay         time.Duration
	lastErrorOnly bool
}

// Retrier
type Retrier struct {
	config *Config

	// Backoff specifies the policy for how long to wait between retries
	Backoff Backoff

	RetryWaitMin time.Duration // Minimum time to wait
	RetryWaitMax time.Duration // Maximum time to wait
	RetryMax     int           // Maximum number of retries

	// CheckRetry specifies the policy for handling retries, and is called
	// after each request. The default policy is DefaultRetryPolicy.
	CheckRetry CheckRetry
}

func NewRetrier(opts ...Option) *Retrier {
	//default
	config := &Config{
		delay:         100 * time.Millisecond,
		lastErrorOnly: false,
	}

	//apply opts
	for _, opt := range opts {
		opt(config)
	}

	return &Retrier{
		config:     config,
		CheckRetry: DefaultRetryPolicy,
		Backoff:    DefaultBackoff,
	}
}

func (r *Retrier) Do(c *circuit, req *Request) (*http.Response, error) {
	var code int // HTTP response code
	var resp *http.Response
	var err error

	for i := 0; ; i++ {
		// Always rewind the request body when non-nil.
		if req.Body != nil {
			body, err := req.Body()
			if err != nil {
				return resp, err
			}
			if c, ok := body.(io.ReadCloser); ok {
				req.Request.Body = c
			} else {
				req.Request.Body = ioutil.NopCloser(body)
			}
		}

		resp, err = c.RoundTripper.RoundTrip(req.Request)
		if resp != nil {
			code = resp.StatusCode
		}

		// Check if we should continue with retries.
		checkOK, checkErr := r.CheckRetry(req.Context(), resp, err)

		if err != nil {
			log.Printf("[ERR] %s %s request failed: %v", req.Method, req.URL, err)
		}

		// Now decide if we should continue.
		if !checkOK {
			if checkErr != nil {
				err = checkErr
			}
			return resp, err
		}

		// We do this before drainBody beause there's no need for the I/O if
		// we're breaking out
		remain := r.RetryMax - i
		if remain <= 0 {
			break
		}

		// We're going to retry, consume any response to reuse the connection.
		if err == nil && resp != nil {
			c.drainBody(resp.Body)
		}

		wait := r.Backoff(r.RetryWaitMin, r.RetryWaitMax, i, resp)
		desc := fmt.Sprintf("%s %s", req.Method, req.URL)
		if code > 0 {
			desc = fmt.Sprintf("%s (status: %d)", desc, code)
		}

		log.Printf("[DEBUG] %s: retrying in %s (%d left)", desc, wait, remain)

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}
	}

	//if c.ErrorHandler != nil {
	//	c.HTTPClient.CloseIdleConnections()
	//	return c.ErrorHandler(resp, err, c.RetryMax+1)
	//}

	// By default, we close the response body and return an error without
	// returning the response
	if resp != nil {
		resp.Body.Close()
	}
	return nil, fmt.Errorf("%s %s giving up after %d attempts",
		req.Method, req.URL, r.RetryMax+1)
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
