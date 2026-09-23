package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	countryCodePattern  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// dateLayout is the accepted format for the from/to query parameters
// (date-only, no time component — analytics filters operate on whole
// days).
const dateLayout = "2006-01-02"

// validStatuses mirrors payments-service's TransactionStatus values.
var validStatuses = map[string]bool{
	StatusPending:   true,
	StatusCompleted: true,
	StatusFailed:    true,
}

// Filter carries the shared query parameters every analytics endpoint
// accepts. Zero values mean "no filter on this dimension".
type Filter struct {
	From                time.Time
	To                  time.Time
	SourceCountry       string
	DestinationCountry  string
	SourceCurrency      string
	DestinationCurrency string
	Provider            string
	Status              string
}

// ParseFilter builds and validates a Filter from raw query-parameter
// strings (all optional — pass "" for any parameter not supplied).
// Returned errors are field-keyed, matching the ValidationError shape
// used elsewhere in this system's APIs.
func ParseFilter(from, to, sourceCountry, destinationCountry, sourceCurrency, destinationCurrency, provider, status string) (Filter, error) {
	fields := map[string]string{}
	var f Filter

	if from != "" {
		t, err := time.Parse(dateLayout, from)
		if err != nil {
			fields["from"] = fmt.Sprintf("must be a date in YYYY-MM-DD format, got %q", from)
		} else {
			f.From = t
		}
	}

	if to != "" {
		t, err := time.Parse(dateLayout, to)
		if err != nil {
			fields["to"] = fmt.Sprintf("must be a date in YYYY-MM-DD format, got %q", to)
		} else {
			// "to" is inclusive of the whole day, so the upper bound
			// used in queries is the start of the *next* day.
			f.To = t.AddDate(0, 0, 1)
		}
	}

	if !f.From.IsZero() && !f.To.IsZero() && !f.From.Before(f.To) {
		fields["to"] = "must be after \"from\""
	}

	if sourceCountry != "" {
		if !countryCodePattern.MatchString(sourceCountry) {
			fields["source_country"] = fmt.Sprintf("must be a 2-letter ISO 3166-1 country code, got %q", sourceCountry)
		} else {
			f.SourceCountry = sourceCountry
		}
	}

	if destinationCountry != "" {
		if !countryCodePattern.MatchString(destinationCountry) {
			fields["destination_country"] = fmt.Sprintf("must be a 2-letter ISO 3166-1 country code, got %q", destinationCountry)
		} else {
			f.DestinationCountry = destinationCountry
		}
	}

	if sourceCurrency != "" {
		if !currencyCodePattern.MatchString(sourceCurrency) {
			fields["source_currency"] = fmt.Sprintf("must be a 3-letter ISO 4217 currency code, got %q", sourceCurrency)
		} else {
			f.SourceCurrency = sourceCurrency
		}
	}

	if destinationCurrency != "" {
		if !currencyCodePattern.MatchString(destinationCurrency) {
			fields["destination_currency"] = fmt.Sprintf("must be a 3-letter ISO 4217 currency code, got %q", destinationCurrency)
		} else {
			f.DestinationCurrency = destinationCurrency
		}
	}

	if provider != "" {
		f.Provider = provider
	}

	if status != "" {
		if !validStatuses[status] {
			fields["status"] = fmt.Sprintf("must be one of pending, completed, failed, got %q", status)
		} else {
			f.Status = status
		}
	}

	if len(fields) > 0 {
		return Filter{}, &ValidationError{Fields: fields}
	}
	return f, nil
}

// CountryDirection selects which side of a transaction "volume by
// country" aggregates over.
type CountryDirection string

const (
	CountryDirectionSource      CountryDirection = "source"
	CountryDirectionDestination CountryDirection = "destination"
)

// ParseCountryDirection validates the `by` query parameter for the
// countries endpoint, defaulting to "source" when empty.
func ParseCountryDirection(raw string) (CountryDirection, error) {
	switch strings.ToLower(raw) {
	case "", string(CountryDirectionSource):
		return CountryDirectionSource, nil
	case string(CountryDirectionDestination):
		return CountryDirectionDestination, nil
	default:
		return "", &ValidationError{Fields: map[string]string{
			"by": fmt.Sprintf("must be %q or %q, got %q", CountryDirectionSource, CountryDirectionDestination, raw),
		}}
	}
}

// TimeInterval selects the bucketing granularity for the time-series
// endpoint. Only "day" is supported today; the type exists so adding
// "hour"/"month" later is a small addition to this switch and the
// repository's query builder, not a redesign.
type TimeInterval string

const TimeIntervalDay TimeInterval = "day"

// ParseTimeInterval validates the `interval` query parameter, defaulting
// to "day" when empty.
func ParseTimeInterval(raw string) (TimeInterval, error) {
	switch strings.ToLower(raw) {
	case "", string(TimeIntervalDay):
		return TimeIntervalDay, nil
	default:
		return "", &ValidationError{Fields: map[string]string{
			"interval": fmt.Sprintf("must be %q, got %q", TimeIntervalDay, raw),
		}}
	}
}

// ValidationError reports one or more invalid input fields.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %v", e.Fields)
}
