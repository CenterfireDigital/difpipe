package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// Policy defines retry behavior
type Policy struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
	Jitter      bool
}

// DefaultPolicy returns a default retry policy
func DefaultPolicy() *Policy {
	return &Policy{
		MaxAttempts: 3,
		InitialWait: 1 * time.Second,
		MaxWait:     30 * time.Second,
		Multiplier:  2.0,
		Jitter:      true,
	}
}

// ExponentialPolicy returns an exponential backoff policy
func ExponentialPolicy(maxAttempts int) *Policy {
	return &Policy{
		MaxAttempts: maxAttempts,
		InitialWait: 1 * time.Second,
		MaxWait:     60 * time.Second,
		Multiplier:  2.0,
		Jitter:      true,
	}
}

// LinearPolicy returns a linear backoff policy
func LinearPolicy(maxAttempts int, wait time.Duration) *Policy {
	return &Policy{
		MaxAttempts: maxAttempts,
		InitialWait: wait,
		MaxWait:     wait,
		Multiplier:  1.0,
		Jitter:      false,
	}
}

// Result contains information about a retry attempt
type Result struct {
	Attempt       int
	Success       bool
	Error         error
	TotalDuration time.Duration
	Attempts      []AttemptInfo
}

// AttemptInfo contains information about a single attempt
type AttemptInfo struct {
	Attempt  int
	Error    error
	Duration time.Duration
	WaitTime time.Duration
}

// Do executes fn with retry logic according to the policy
func Do(ctx context.Context, policy *Policy, fn func() error) *Result {
	startTime := time.Now()
	result := &Result{
		Attempts: make([]AttemptInfo, 0, policy.MaxAttempts),
	}

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		result.Attempt = attempt
		attemptStart := time.Now()

		// Execute function
		err := fn()
		attemptDuration := time.Since(attemptStart)

		info := AttemptInfo{
			Attempt:  attempt,
			Error:    err,
			Duration: attemptDuration,
		}

		// Success
		if err == nil {
			result.Success = true
			result.TotalDuration = time.Since(startTime)
			result.Attempts = append(result.Attempts, info)
			return result
		}

		result.Error = err

		// Check if we should retry
		if attempt >= policy.MaxAttempts {
			result.Attempts = append(result.Attempts, info)
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			result.Error = ctx.Err()
			result.Attempts = append(result.Attempts, info)
			break
		}

		// Calculate wait time
		waitTime := policy.calculateWait(attempt)
		info.WaitTime = waitTime
		result.Attempts = append(result.Attempts, info)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.TotalDuration = time.Since(startTime)
			return result
		case <-time.After(waitTime):
			// Continue to next attempt
		}
	}

	result.TotalDuration = time.Since(startTime)
	return result
}

// DoWithRetryable executes fn with retry logic, but only retries if isRetryable returns true
func DoWithRetryable(ctx context.Context, policy *Policy, fn func() error, isRetryable func(error) bool) *Result {
	startTime := time.Now()
	result := &Result{
		Attempts: make([]AttemptInfo, 0, policy.MaxAttempts),
	}

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		result.Attempt = attempt
		attemptStart := time.Now()

		// Execute function
		err := fn()
		attemptDuration := time.Since(attemptStart)

		info := AttemptInfo{
			Attempt:  attempt,
			Error:    err,
			Duration: attemptDuration,
		}

		// Success
		if err == nil {
			result.Success = true
			result.TotalDuration = time.Since(startTime)
			result.Attempts = append(result.Attempts, info)
			return result
		}

		result.Error = err

		// Check if error is retryable
		if !isRetryable(err) {
			result.Attempts = append(result.Attempts, info)
			result.TotalDuration = time.Since(startTime)
			return result
		}

		// Check if we should retry
		if attempt >= policy.MaxAttempts {
			result.Attempts = append(result.Attempts, info)
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			result.Error = ctx.Err()
			result.Attempts = append(result.Attempts, info)
			break
		}

		// Calculate wait time
		waitTime := policy.calculateWait(attempt)
		info.WaitTime = waitTime
		result.Attempts = append(result.Attempts, info)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.TotalDuration = time.Since(startTime)
			return result
		case <-time.After(waitTime):
			// Continue to next attempt
		}
	}

	result.TotalDuration = time.Since(startTime)
	return result
}

// calculateWait calculates the wait time for a given attempt
func (p *Policy) calculateWait(attempt int) time.Duration {
	// Calculate exponential backoff
	wait := float64(p.InitialWait) * math.Pow(p.Multiplier, float64(attempt-1))

	// Cap at max wait
	if wait > float64(p.MaxWait) {
		wait = float64(p.MaxWait)
	}

	// Add jitter if enabled
	if p.Jitter {
		jitter := rand.Float64() * wait * 0.1 // 10% jitter
		wait += jitter
	}

	return time.Duration(wait)
}

// String returns a string representation of the result
func (r *Result) String() string {
	if r.Success {
		return fmt.Sprintf("Success after %d attempt(s) in %v", r.Attempt, r.TotalDuration)
	}
	return fmt.Sprintf("Failed after %d attempt(s) in %v: %v", r.Attempt, r.TotalDuration, r.Error)
}
