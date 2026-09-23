package domain

import "errors"

// ErrRepository indicates a persistence-layer failure. Concrete
// repository implementations wrap the underlying driver error with this
// sentinel so callers can detect "something went wrong talking to
// ClickHouse" without leaking driver-specific error types.
var ErrRepository = errors.New("repository error")

// AsValidationError reports whether err is (or wraps) a *ValidationError
// and returns it.
func AsValidationError(err error) (*ValidationError, bool) {
	var verr *ValidationError
	if errors.As(err, &verr) {
		return verr, true
	}
	return nil, false
}
