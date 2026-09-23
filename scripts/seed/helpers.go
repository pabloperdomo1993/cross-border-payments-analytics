package main

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func parseDecimal(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}
