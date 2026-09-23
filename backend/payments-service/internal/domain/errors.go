package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Sentinel errors the application and HTTP layers can match on with
// errors.Is, independent of how the underlying failure was produced
// (domain validation, repository lookup, etc).
var (
	// ErrTransactionNotFound indicates no transaction exists for a
	// given ID.
	ErrTransactionNotFound = errors.New("transaction not found")

	// ErrConflict indicates the requested write conflicts with existing
	// state (e.g. a duplicate ID).
	ErrConflict = errors.New("transaction conflict")

	// ErrRepository indicates a persistence-layer failure. Concrete
	// repository implementations wrap the underlying driver error with
	// this sentinel so callers can detect "something went wrong
	// talking to the database" without leaking driver/SQL details.
	ErrRepository = errors.New("repository error")

	// ErrInvalidTransition indicates an attempt to move a transaction
	// to a status it cannot reach from its current status.
	ErrInvalidTransition = errors.New("invalid transaction status transition")
)

// ValidationError reports one or more invalid input fields. HTTP
// handlers map it to 400 Bad Request and surface Fields directly to the
// client.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, e.Fields[k]))
	}
	return "validation error: " + strings.Join(parts, "; ")
}

// AsValidationError reports whether err is (or wraps) a *ValidationError
// and returns it.
func AsValidationError(err error) (*ValidationError, bool) {
	var verr *ValidationError
	if errors.As(err, &verr) {
		return verr, true
	}
	return nil, false
}
