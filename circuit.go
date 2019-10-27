package gcb

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

var (
	_ http.RoundTripper = (*circuit)(nil)

	// defaultLogger is the logger provided with defaultClient
	defaultLogger = log.New(os.Stderr, "", log.LstdFlags)

	// We need to consume response bodies to maintain http connections, but
	// limit the size we consume to respReadLimit.
	respReadLimit = int64(4096)
)

// ErrorHandler is called if retries are expired, containing the last status
// from the http library. If not specified, default behavior for the library is
// to close the body and return an error indicating how many tries were
// attempted. If overriding this, be sure to close the body if needed.
type ErrorHandler func(resp *http.Response, err error, numTries int) (*http.Response, error)

type State int8

const (
	Open State = iota + 1
	HalfOpen
	Close
)

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

// circuit
type circuit struct {
	state   State
	retrier *Retrier

	RoundTripper http.RoundTripper

	// CheckRetry specifies the policy for handling retries, and is called
	// after each request. The default policy is DefaultRetryPolicy.
	CheckRetry CheckRetry

	// ErrorHandler specifies the custom error handler to use, if any
	ErrorHandler ErrorHandler
}

func NewCircuit() *circuit {
	retrier := NewRetrier()
	return &circuit{
		retrier:      retrier,
		RoundTripper: http.DefaultTransport,
		CheckRetry:   DefaultRetryPolicy,
	}
}

// ReaderFunc is the type of function that can be given natively to NewRequest
type ReaderFunc func() (io.Reader, error)

// Request wraps the metadata needed to create HTTP requests.
type Request struct {
	// body is a seekable reader over the request body payload. This is
	// used to rewind the request data in between retries.
	Body ReaderFunc

	// Embed an HTTP request directly. This makes a *Request act exactly
	// like an *http.Request so that all meta methods are supported.
	*http.Request
}

// NewRequest creates a new wrapped request.
func NewRequest(method, url string, rawBody interface{}) (*Request, error) {
	bodyReader, contentLength, err := getBodyReaderAndContentLength(rawBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.ContentLength = contentLength

	return &Request{bodyReader, httpReq}, nil
}

// LenReader is an interface implemented by many in-memory io.Reader's. Used
// for automatically sending the right Content-Length header when possible.
type LenReader interface {
	Len() int
}

func (c *circuit) RoundTrip(req *http.Request) (*http.Response, error) {
	r, err := NewRequest(req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	return c.retrier.Do(c, r)
}



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
				c.Close()
			}

		case func() (io.Reader, error):
			bodyReader = body
			tmp, err := body()
			if err != nil {
				return nil, 0, err
			}
			if lr, ok := tmp.(LenReader); ok {
				contentLength = int64(lr.Len())
			}
			if c, ok := tmp.(io.Closer); ok {
				c.Close()
			}

		// If a regular byte slice, we can read it over and over via new
		// readers
		case []byte:
			buf := body
			bodyReader = func() (io.Reader, error) {
				return bytes.NewReader(buf), nil
			}
			contentLength = int64(len(buf))

		// If a bytes.Buffer we can read the underlying byte slice over and
		// over
		case *bytes.Buffer:
			buf := body
			bodyReader = func() (io.Reader, error) {
				return bytes.NewReader(buf.Bytes()), nil
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
			bodyReader = func() (io.Reader, error) {
				return bytes.NewReader(buf), nil
			}
			contentLength = int64(len(buf))

		// Compat case
		case io.ReadSeeker:
			raw := body
			bodyReader = func() (io.Reader, error) {
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
			bodyReader = func() (io.Reader, error) {
				return bytes.NewReader(buf), nil
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
