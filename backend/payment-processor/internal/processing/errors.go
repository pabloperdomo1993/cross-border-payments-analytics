package processing

import (
	"errors"
	"fmt"
)

// Sentinel errors describing why processing of a PaymentEvent failed.
// All of these are non-retryable: they indicate the event itself is
// unprocessable, not that a downstream dependency hiccuped.
var (
	ErrMalformedEvent      = errors.New("malformed payment event")
	ErrInvalidPaymentData  = errors.New("invalid payment data")
	ErrUnsupportedCurrency = errors.New("unsupported currency")

	// ErrPublishFailed indicates the processed outcome could not be
	// published downstream (e.g. a transient Kafka broker issue).
	// Unlike the sentinels above, this is retryable.
	ErrPublishFailed = errors.New("failed to publish processing outcome")
)

// ProcessingError wraps a processing failure with an explicit
// Retryable flag, so callers (the Kafka consumer's retry loop) don't
// need to know which sentinel maps to which behavior.
type ProcessingError struct {
	Err       error
	Retryable bool
}

func (e *ProcessingError) Error() string {
	return fmt.Sprintf("%s (retryable=%t)", e.Err, e.Retryable)
}

func (e *ProcessingError) Unwrap() error {
	return e.Err
}

// NonRetryable wraps err as a non-retryable ProcessingError.
func NonRetryable(err error) *ProcessingError {
	return &ProcessingError{Err: err, Retryable: false}
}

// Retryable wraps err as a retryable ProcessingError.
func Retryable(err error) *ProcessingError {
	return &ProcessingError{Err: err, Retryable: true}
}

// IsRetryable reports whether err (or a wrapped *ProcessingError within
// it) is marked retryable. A plain error that isn't a *ProcessingError
// is treated as non-retryable, since only classified errors are known
// to be safe to retry.
func IsRetryable(err error) bool {
	var pErr *ProcessingError
	if errors.As(err, &pErr) {
		return pErr.Retryable
	}
	return false
}
