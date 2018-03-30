// Copyright 2016-2018, Pulumi Corporation.  All rights reserved.

package retry

import (
	"context"
	"time"
)

type Acceptor struct {
	Accept   Acceptance     // a function that determines when to proceed.
	Delay    *time.Duration // an optional delay duration.
	Backoff  *float64       // an optional backoff multiplier.
	MaxDelay *time.Duration // an optional maximum delay duration.
}

// Acceptance is meant to accept a condition.  It returns true when this condition has succeeded, and false otherwise
// (to which the retry framework responds by waiting and retrying after a certain period of time).  If a non-nil error
// is returned, retrying halts.  The interface{} data may be used to return final values to the caller.
type Acceptance func(try int, nextRetryTime time.Duration) (bool, interface{}, error)

const (
	DefaultDelay    time.Duration = 100 * time.Millisecond // by default, delay by 100ms
	DefaultBackoff  float64       = 1.5                    // by default, backoff by 1.5x
	DefaultMaxDelay time.Duration = 5 * time.Second        // by default, no more than 5 seconds
)

// Until waits until the acceptor accepts the current condition, or the context expires, whichever comes first.  A
// return boolean of true means the acceptor eventually accepted; a non-nil error means the acceptor returned an error.
// If an acceptor accepts a condition after the context has expired, we ignore the expiration and return the condition.
func Until(ctx context.Context, acceptor Acceptor) (bool, interface{}, error) {
	expired := false

	// Prepare our delay and backoff variables.
	var delay time.Duration
	if acceptor.Delay == nil {
		delay = DefaultDelay
	} else {
		delay = *acceptor.Delay
	}
	var backoff float64
	if acceptor.Backoff == nil {
		backoff = DefaultBackoff
	} else {
		backoff = *acceptor.Backoff
	}
	var maxDelay time.Duration
	if acceptor.MaxDelay == nil {
		maxDelay = DefaultMaxDelay
	} else {
		maxDelay = *acceptor.MaxDelay
	}

	// If the context expires before the waiter has accepted, return.
	if ctx != nil {
		go func() {
			<-ctx.Done()
			expired = true
		}()
	}

	// Loop until the condition is accepted, or the context expires, whichever comes first.
	var try int
	for !expired {
		// Compute the next retry time so the callback can access it.
		delay = time.Duration(float64(delay) * backoff)
		if delay > maxDelay {
			delay = maxDelay
		}

		// Try the acceptance condition; if it returns true, or an error, we are done.
		b, data, err := acceptor.Accept(try, delay)
		if b || err != nil {
			return b, data, err
		}

		// About to try again.  Sleep, bump the retry count, and go around the horn again.
		time.Sleep(delay)
		try++
	}

	return false, nil, nil
}

// UntilDeadline creates a child context with the given deadline, and then invokes the above Until function.
func UntilDeadline(ctx context.Context, acceptor Acceptor, deadline time.Time) (bool, interface{}, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithDeadline(ctx, deadline)
	b, data, err := Until(ctx, acceptor)
	cancel()
	return b, data, err
}

// UntilTimeout creates a child context with the given timeout, and then invokes the above Until function.
func UntilTimeout(ctx context.Context, acceptor Acceptor, timeout time.Duration) (bool, interface{}, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	b, data, err := Until(ctx, acceptor)
	cancel()
	return b, data, err
}
