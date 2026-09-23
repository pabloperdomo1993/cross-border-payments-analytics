// Package application contains the payments-service use cases. It
// orchestrates domain objects and the repository interface, and knows
// nothing about HTTP or any specific database technology.
package application

import (
	"crypto/rand"
	"fmt"
)

// NewTransactionID generates a random UUIDv4-shaped identifier for a new
// Transaction. It uses only crypto/rand (stdlib) rather than a UUID
// library, since generating a random, RFC 4122-shaped string is all
// that's required here.
func NewTransactionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate transaction id: %w", err)
	}

	// Set version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
