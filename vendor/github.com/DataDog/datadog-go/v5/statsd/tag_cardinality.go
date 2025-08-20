package statsd

import (
	"os"
	"strings"
	"sync"
)

type Parameter interface{}

type Cardinality int

const (
	CardinalityNotSet Cardinality = -1
	CardinalityNone   Cardinality = iota
	CardinalityLow
	CardinalityOrchestrator
	CardinalityHigh
)

func (c Cardinality) String() string {
	switch c {
	case CardinalityNone:
		return "none"
	case CardinalityLow:
		return "low"
	case CardinalityOrchestrator:
		return "orchestrator"
	case CardinalityHigh:
		return "high"
	}
	return ""
}

// validateCardinality converts a string to Cardinality
func validateCardinality(card string) Cardinality {
	card = strings.ToLower(card)
	switch card {
	case "none":
		return CardinalityNone
	case "low":
		return CardinalityLow
	case "orchestrator":
		return CardinalityOrchestrator
	case "high":
		return CardinalityHigh
	default:
		return CardinalityNotSet
	}
}

var (
	// Global setting of the tag cardinality.
	tagCardinality      Cardinality = CardinalityNotSet
	tagCardinalityMutex sync.RWMutex
)

// initTagCardinality initializes the tag cardinality.
func initTagCardinality(card Cardinality) {
	tagCardinalityMutex.Lock()
	defer tagCardinalityMutex.Unlock()

	tagCardinality = card

	// If the user has not provided a valid value, read the value from the DD_CARDINALITY environment variable.
	if tagCardinality.String() == "" {
		tagCardinality = validateCardinality(os.Getenv("DD_CARDINALITY"))
	}
	// If DD_CARDINALITY is not set or valid, read the value from the DATADOG_CARDINALITY environment variable.
	if tagCardinality.String() == "" {
		tagCardinality = validateCardinality(os.Getenv("DATADOG_CARDINALITY"))
	}
}

func getTagCardinality() Cardinality {
	tagCardinalityMutex.RLock()
	defer tagCardinalityMutex.RUnlock()
	return tagCardinality
}

func parseTagCardinality(parameters []Parameter) Cardinality {
	cardinality := CardinalityNotSet
	for _, o := range parameters {
		c, ok := o.(Cardinality)
		if ok {
			cardinality = c
		}
	}
	return resolveCardinality(cardinality)
}

// resolveCardinality returns the cardinality to use, prioritizing the metric-level cardinality over the global setting.
// This function validates the cardinality and falls back to the global setting if invalid.
func resolveCardinality(card Cardinality) Cardinality {
	if card.String() == "" {
		return getTagCardinality()
	}
	return card
}
