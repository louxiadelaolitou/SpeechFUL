// Package postthumbnailcache owns the optional, discardable Redis copy of
// protected post thumbnails. PostgreSQL remains the source of truth; every
// value is revalidated against PostgreSQL metadata before it can be released.
package postthumbnailcache

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	// The namespace is fixed per environment. Cross-environment credentials or
	// key spaces are rejected by validation, not just by convention.
	namespacePrefix = "hnuhole:post-thumbnail:v1:"

	DefaultTTLSeconds                   = 300
	DefaultDeadlineMilliseconds         = 10
	DefaultFillQueueEntries             = 1024
	DefaultFillQueueBytes               = 32 << 20
	DefaultDeleteBatch                  = 32
	DefaultCircuitFailures              = 3
	DefaultCircuitCooldownMilliseconds  = 5000
	minTTLSeconds                       = 30
	maxTTLSeconds                       = 3600
	maxDeadlineMilliseconds             = 1000
	maxFillQueueEntries                 = 65536
	minFillQueueBytes                   = 1 << 20
	maxFillQueueBytes                   = 256 << 20
	maxDeleteBatch                      = 256
	maxCircuitFailures                  = 100
	maxCircuitCooldownMilliseconds      = 60000
	minCircuitCooldownMilliseconds      = 100
	maxRedisDatabase                    = 15
	namespaceEnvironmentSeparatorSuffix = ":"
)

var errConfig = errors.New("invalid post thumbnail cache configuration")

// Config is the explicit opt-in for the independent thumbnail Redis instance.
// It never shares the comment-snapshot or shared-admission instance.
type Config struct {
	Address                     string `json:"address"`
	Username                    string `json:"username"`
	PasswordFile                string `json:"passwordFile"`
	Namespace                   string `json:"namespace"`
	Database                    int    `json:"database,omitempty"`
	TTLSeconds                  int    `json:"ttlSeconds,omitempty"`
	DeadlineMilliseconds        int    `json:"deadlineMilliseconds,omitempty"`
	FillQueueEntries            int    `json:"fillQueueEntries,omitempty"`
	FillQueueBytes              int64  `json:"fillQueueBytes,omitempty"`
	DeleteBatch                 int    `json:"deleteBatch,omitempty"`
	CircuitFailures             int    `json:"circuitFailures,omitempty"`
	CircuitCooldownMilliseconds int    `json:"circuitCooldownMilliseconds,omitempty"`
}

// Effective fills the documented experimental defaults. Zero is the only
// value treated as "unset"; negative or out-of-range values are rejected.
func (c Config) Effective() Config {
	if c.TTLSeconds == 0 {
		c.TTLSeconds = DefaultTTLSeconds
	}
	if c.DeadlineMilliseconds == 0 {
		c.DeadlineMilliseconds = DefaultDeadlineMilliseconds
	}
	if c.FillQueueEntries == 0 {
		c.FillQueueEntries = DefaultFillQueueEntries
	}
	if c.FillQueueBytes == 0 {
		c.FillQueueBytes = DefaultFillQueueBytes
	}
	if c.DeleteBatch == 0 {
		c.DeleteBatch = DefaultDeleteBatch
	}
	if c.CircuitFailures == 0 {
		c.CircuitFailures = DefaultCircuitFailures
	}
	if c.CircuitCooldownMilliseconds == 0 {
		c.CircuitCooldownMilliseconds = DefaultCircuitCooldownMilliseconds
	}
	return c
}

// Validate binds the namespace to one environment and bounds every runtime
// resource. It does not read credentials or contact Redis.
func (c Config) Validate(environment string) error {
	c = c.Effective()
	if environment == "" || strings.ContainsAny(environment, ": \t\r\n") {
		return fmt.Errorf("%w: environment", errConfig)
	}
	want := namespacePrefix + environment + namespaceEnvironmentSeparatorSuffix
	if c.Namespace != want {
		return fmt.Errorf("%w: namespace must be %q", errConfig, want)
	}
	return c.validateRuntime()
}

// validateRuntime checks the parts New can verify without an environment
// binding. Validate additionally requires the namespace to match environment.
func (c Config) validateRuntime() error {
	host, port, err := net.SplitHostPort(c.Address)
	if err != nil || host == "" || port == "" || strings.ContainsAny(c.Address, " \t\r\n") {
		return fmt.Errorf("%w: address must be host:port", errConfig)
	}
	if c.Username == "" || strings.ContainsAny(c.Username, " \t\r\n") {
		return fmt.Errorf("%w: username", errConfig)
	}
	if c.PasswordFile == "" {
		return fmt.Errorf("%w: passwordFile", errConfig)
	}
	if c.Database < 0 || c.Database > maxRedisDatabase {
		return fmt.Errorf("%w: database", errConfig)
	}
	if c.TTLSeconds < minTTLSeconds || c.TTLSeconds > maxTTLSeconds {
		return fmt.Errorf("%w: ttlSeconds", errConfig)
	}
	if c.DeadlineMilliseconds < 1 || c.DeadlineMilliseconds > maxDeadlineMilliseconds {
		return fmt.Errorf("%w: deadlineMilliseconds", errConfig)
	}
	if c.FillQueueEntries < 1 || c.FillQueueEntries > maxFillQueueEntries {
		return fmt.Errorf("%w: fillQueueEntries", errConfig)
	}
	if c.FillQueueBytes < minFillQueueBytes || c.FillQueueBytes > maxFillQueueBytes {
		return fmt.Errorf("%w: fillQueueBytes", errConfig)
	}
	if c.DeleteBatch < 1 || c.DeleteBatch > maxDeleteBatch {
		return fmt.Errorf("%w: deleteBatch", errConfig)
	}
	if c.CircuitFailures < 1 || c.CircuitFailures > maxCircuitFailures {
		return fmt.Errorf("%w: circuitFailures", errConfig)
	}
	if c.CircuitCooldownMilliseconds < minCircuitCooldownMilliseconds || c.CircuitCooldownMilliseconds > maxCircuitCooldownMilliseconds {
		return fmt.Errorf("%w: circuitCooldownMilliseconds", errConfig)
	}
	return nil
}
