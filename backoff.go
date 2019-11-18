//  A backoff strategy specifies how long to wait between shouldRetry.
//  Similar to the retry policy, export a function type and provide a default implementation
//  of the same.
package gcb

import (
	"math"
	"math/rand"
	"net/http"
	"time"
)

type (
	// BackoffStrategy is used to determine how long a retry request should wait until attempted
	BackoffStrategy func(retry int) time.Duration
	// Backoff specifies a policy for how long to wait between shouldRetry.
	// It is called after a failing request to determine the amount of time
	// that should pass before trying again.
	Backoff func(min, max time.Duration, attemptNum uint32, resp *http.Response) time.Duration

	// BackOff is a backoff policy for retrying an operation.
	BackOff interface {
		// NextBackOff returns the duration to wait before retrying the operation,
		// or backoff. Stop to indicate that no more shouldRetry should be made.
		//
		// Example usage:
		//
		// 	duration := backoff.NextBackOff();
		// 	if (duration == backoff.Stop) {
		// 		// Do not retry operation.
		// 	} else {
		// 		// Sleep for duration and retry operation.
		// 	}
		//
		NextBackOff() time.Duration

		// Reset to initial state.
		Reset()
	}
)

// DefaultBackoff provides a default callback for Client.Backoff which
// will perform exponential backoff based on the attempt number and limited
// by the provided minimum and maximum durations.
func DefaultBackoff(min, max time.Duration, attemptNum uint32, resp *http.Response) time.Duration {
	mult := math.Pow(2, float64(attemptNum)) * float64(min)
	sleep := time.Duration(mult)
	if float64(sleep) != mult || sleep > max {
		sleep = max
	}
	return sleep
}

// LinearJitterBackoff provides a callback for Client.Backoff which will
// perform linear backoff based on the attempt number and with jitter to
// prevent a thundering herd.
//
// min and max here are *not* absolute values. The number to be multipled by
// the attempt number will be chosen at random from between them, thus they are
// bounding the jitter.
//
// For instance:
// * To get strictly linear backoff of one second increasing each retry, set
// both to one second (1s, 2s, 3s, 4s, ...)
// * To get a small amount of jitter centered around one second increasing each
// retry, set to around one second, such as a min of 800ms and max of 1200ms
// (892ms, 2102ms, 2945ms, 4312ms, ...)
// * To get extreme jitter, set to a very wide spread, such as a min of 100ms
// and a max of 20s (15382ms, 292ms, 51321ms, 35234ms, ...)
func LinearJitterBackoff(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
	// attemptNum always starts at zero but we want to start at 1 for multiplication
	attemptNum++

	if max <= min {
		// Unclear what to do here, or they are the same, so return min *
		// attemptNum
		return min * time.Duration(attemptNum)
	}

	// Seed rand; doing this every time is fine
	rand := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))

	// Pick a random number that lies somewhere between the min and max and
	// multiply by the attemptNum. attemptNum starts at zero so we always
	// increment here. We first get a random percentage, then apply that to the
	// difference between min and max, and add to min.
	jitter := rand.Float64() * float64(max-min)
	jitterMin := int64(jitter) + int64(min)
	return time.Duration(jitterMin * int64(attemptNum))
}
