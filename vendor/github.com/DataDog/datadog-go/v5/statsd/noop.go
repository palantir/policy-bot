package statsd

import "time"

// NoOpClient is a statsd client that does nothing. Can be useful in testing
// situations for library users.
type NoOpClient struct{}

// Gauge does nothing and returns nil
func (n *NoOpClient) Gauge(name string, value float64, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// GaugeWithTimestamp does nothing and returns nil
func (n *NoOpClient) GaugeWithTimestamp(name string, value float64, tags []string, rate float64, timestamp time.Time, parameters ...Parameter) error {
	return nil
}

// Count does nothing and returns nil
func (n *NoOpClient) Count(name string, value int64, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// CountWithTimestamp does nothing and returns nil
func (n *NoOpClient) CountWithTimestamp(name string, value int64, tags []string, rate float64, timestamp time.Time, parameters ...Parameter) error {
	return nil
}

// Histogram does nothing and returns nil
func (n *NoOpClient) Histogram(name string, value float64, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// Distribution does nothing and returns nil
func (n *NoOpClient) Distribution(name string, value float64, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// Decr does nothing and returns nil
func (n *NoOpClient) Decr(name string, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// Incr does nothing and returns nil
func (n *NoOpClient) Incr(name string, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// Set does nothing and returns nil
func (n *NoOpClient) Set(name string, value string, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// Timing does nothing and returns nil
func (n *NoOpClient) Timing(name string, value time.Duration, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// TimeInMilliseconds does nothing and returns nil
func (n *NoOpClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64, parameters ...Parameter) error {
	return nil
}

// Event does nothing and returns nil
func (n *NoOpClient) Event(e *Event, parameters ...Parameter) error {
	return nil
}

// SimpleEvent does nothing and returns nil
func (n *NoOpClient) SimpleEvent(title, text string, parameters ...Parameter) error {
	return nil
}

// ServiceCheck does nothing and returns nil
func (n *NoOpClient) ServiceCheck(sc *ServiceCheck, parameters ...Parameter) error {
	return nil
}

// SimpleServiceCheck does nothing and returns nil
func (n *NoOpClient) SimpleServiceCheck(name string, status ServiceCheckStatus, parameters ...Parameter) error {
	return nil
}

// Close does nothing and returns nil
func (n *NoOpClient) Close() error {
	return nil
}

// Flush does nothing and returns nil
func (n *NoOpClient) Flush() error {
	return nil
}

// IsClosed does nothing and return false
func (n *NoOpClient) IsClosed() bool {
	return false
}

// GetTelemetry does nothing and returns an empty Telemetry
func (n *NoOpClient) GetTelemetry() Telemetry {
	return Telemetry{}
}

// Verify that NoOpClient implements the ClientInterface.
// https://golang.org/doc/faq#guarantee_satisfies_interface
var _ ClientInterface = &NoOpClient{}

// NoOpClientDirect implements ClientDirectInterface and does nothing.
type NoOpClientDirect struct {
	NoOpClient
}

// DistributionSamples does nothing and returns nil
func (n *NoOpClientDirect) DistributionSamples(name string, values []float64, tags []string, rate float64) error {
	return nil
}

var _ ClientDirectInterface = &NoOpClientDirect{}
