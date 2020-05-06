package gcb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var (
	// makes sure circuit implements the round tripper interface
	_ http.RoundTripper = (*circuit)(nil)
	// We need to consume response bodies to maintain http connections, but
	// limit the size we consume to respReadLimit.
	respReadLimit = int64(4096)

	rateLimitExceeded = errors.New("exceeded rate limit")
)

type (
	// TODO: can be removed?
	// ErrorHandler is called if shouldRetry are expired, containing the last status
	// from the http library. If not specified, default behavior for the library is
	// to close the body and return an error indicating how many tries were
	// attempted. If overriding this, be sure to close the body if needed.
	ErrorHandler func(resp *http.Response, err error, numTries int) (*http.Response, error)

	// ReaderFunc is the type of function that can be given natively to newRequest
	ReaderFunc func() (io.ReadCloser, error)

	// LenReader is an interface implemented by many in-memory io.Reader's. Used
	// for automatically sending the right Content-Length header when possible.
	LenReader interface {
		Len() int
	}

	// Request wraps the metadata needed to create HTTP requests.
	Request struct {
		// body is a seekable reader over the request body payload. This is
		// used to rewind the request data in between shouldRetry.
		Body ReaderFunc

		// Embed an HTTP request directly. This makes a *Request act exactly
		// like an *http.Request so that all meta methods are supported.
		*http.Request
	}

	circuit struct {
		retrier *Retrier
		breaker *Breaker

		RoundTripper http.RoundTripper

		// ErrorHandler specifies the custom error handler to use, if any
		ErrorHandler ErrorHandler
	}
)

func newCircuitBreaker(opts ...Option) *circuit {
	retrier := NewRetrier(opts...)
	breaker := NewBreaker(opts...)
	return &circuit{
		retrier:      retrier,
		breaker:      breaker,
		RoundTripper: http.DefaultTransport,
	}
}

// RoundTrip intercepts the request and takes action from here:
// - retry
// - rate limiting
// - circuit breaking
func (c *circuit) RoundTrip(req *http.Request) (*http.Response, error) {
	// wraps the original request
	//request, err := newRequest(req)
	//if err != nil {
	//	return nil, err
	//}

	// the circuit breaker
	res, err := c.breaker.Execute(func() (*http.Response, error) {
		var code int            // HTTP response code
		var resp *http.Response // HTTP response
		var err error

		// run X times
		var i uint32
		for i = 0; ; i++ {
			resp, err = c.RoundTripper.RoundTrip(req)

			// Check if we should continue with shouldRetry.
			shouldRetry, checkErr := c.retrier.retryPolicy(req.Context(), resp, err)

			// Now decide if we should continue.
			if !shouldRetry {
				if checkErr != nil {
					err = checkErr
				}
				// Depending on the policy, if the request is valid
				// we'll return here
				return resp, err
			}

			// We do this before drainBody because there's no need for the I/O if
			// we're breaking out
			remain := c.retrier.RetryMax - i
			if remain <= 0 {
				err = fmt.Errorf("%s: %s %s giving up after %d attempts", errMaxRetriesReached,
					req.Method, req.URL, c.retrier.RetryMax+1)
				break
			}

			// We're going to retry, consume any response to reuse the connection.
			if err == nil && resp != nil {
				c.drainBody(resp.Body)
			}

			wait := c.retrier.Backoff(c.retrier.RetryWaitMin, c.retrier.RetryWaitMax, i, resp)
			c.logRetry(req, code, wait, remain)

			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(wait):
			}
		}

		return resp, err
	})

	//if c.ErrorHandler != nil {
	//	return c.ErrorHandler(res, err, c.retrier.RetryMax+1)
	//}

	if err == nil {
		_ = res.Body.Close()
	}

	return res, nil
}


func (c *circuit) logRetry(req *http.Request, code int, wait time.Duration, remain uint32) {
	desc := fmt.Sprintf("%s %s", req.Method, req.URL)
	if code > 0 {
		desc = fmt.Sprintf("%s (status: %d)", desc, code)
	}
	log.Printf("[DEBUG] %s: retrying in %s (%d left)", desc, wait, remain)
}


// newRequest creates a new wrapped request.
//func newRequest(method, url string, rawBody io.ReadCloser) (*Request, error) {
//	bodyReader, contentLength, err := getBodyReaderAndContentLength(rawBody)
//	if err != nil {
//		return nil, err
//	}
//
//	httpReq, err := http.NewRequest(method, url, rawBody)
//	if err != nil {
//		return nil, err
//	}
//	httpReq.ContentLength = contentLength
//	httpReq.GetBody = bodyReader
//
//	return &Request{bodyReader, httpReq}, nil
//}

func getBodyReaderAndContentLength(rawBody interface{}) (ReaderFunc, int64, error) {
	var bodyReader ReaderFunc
	var contentLength int64

	if rawBody != nil {
		switch body := rawBody.(type) {
		// If they gave us a function already, great! Use it.
		case ReaderFunc:
			bodyReader = body
			tmp, err := body()
			if err != nil {
				return nil, 0, err
			}
			if lr, ok := tmp.(LenReader); ok {
				contentLength = int64(lr.Len())
			}
			if c, ok := tmp.(io.Closer); ok {
				_ = c.Close()
			}

		case func() (io.Reader, error):
			tmp, err := body()
			bodyReader = func() (io.ReadCloser, error) {
				return ioutil.NopCloser(tmp), nil
			}

			if err != nil {
				return nil, 0, err
			}
			if lr, ok := tmp.(LenReader); ok {
				contentLength = int64(lr.Len())
			}
			if c, ok := tmp.(io.Closer); ok {
				_ = c.Close()
			}

		// If a regular byte slice, we can read it over and over via new
		// readers
		case []byte:
			buf := body
			bodyReader = func() (io.ReadCloser, error) {
				return ioutil.NopCloser(bytes.NewReader(buf)), nil
			}
			contentLength = int64(len(buf))

		// If a bytes.Buffer we can read the underlying byte slice over and
		// over
		case *bytes.Buffer:
			buf := body
			bodyReader = func() (io.ReadCloser, error) {
				return ioutil.NopCloser(bytes.NewReader(buf.Bytes())), nil
			}
			contentLength = int64(buf.Len())

		// We prioritize *bytes.Reader here because we don't really want to
		// deal with it seeking so want it to match here instead of the
		// io.ReadSeeker case.
		case *bytes.Reader:
			buf, err := ioutil.ReadAll(body)
			if err != nil {
				return nil, 0, err
			}
			bodyReader = func() (io.ReadCloser, error) {
				return ioutil.NopCloser(bytes.NewReader(buf)), nil
			}
			contentLength = int64(len(buf))

		// Compat case
		case io.ReadSeeker:
			raw := body
			bodyReader = func() (io.ReadCloser, error) {
				_, err := raw.Seek(0, 0)
				return ioutil.NopCloser(raw), err
			}
			if lr, ok := raw.(LenReader); ok {
				contentLength = int64(lr.Len())
			}

		// Read all in so we can reset
		case io.Reader:
			buf, err := ioutil.ReadAll(body)
			if err != nil {
				return nil, 0, err
			}
			bodyReader = func() (io.ReadCloser, error) {
				readCloser := ioutil.NopCloser(bytes.NewReader(buf))
				return readCloser, nil
			}
			contentLength = int64(len(buf))

		default:
			return nil, 0, fmt.Errorf("cannot handle type %T", rawBody)
		}
	}
	return bodyReader, contentLength, nil
}

// Try to read the response body so we can reuse this connection.
func (c *circuit) drainBody(body io.ReadCloser) {
	defer body.Close()
	_, err := io.Copy(ioutil.Discard, io.LimitReader(body, respReadLimit))
	if err != nil {
		log.Printf("[ERR] error reading response body: %v", err)
	}
}

func (c *circuit) GetState() State {
	return c.breaker.state
}
