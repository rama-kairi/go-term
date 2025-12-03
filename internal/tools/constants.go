package tools

// L1: Constants to replace magic numbers throughout the codebase

const (
	// Default limits
	DefaultSearchLimit = 100
	MaxSearchLimit     = 1000
	DefaultTimeout     = 60  // seconds
	MaxTimeout         = 300 // seconds (5 minutes)

	// Background process defaults
	DefaultBackgroundTimeout = 4 * 60 * 60 // 4 hours in seconds
	MaxBackgroundProcesses   = 3
	BackgroundOutputLimit    = 2000 // characters

	// Rate limiting defaults
	DefaultRateLimitPerMinute = 60
	DefaultRateLimitBurst     = 10

	// Resource monitoring thresholds
	GoroutineLeakThreshold = 100
	MemoryLeakThresholdMB  = 200
	HeapObjectsThreshold   = 500000
	HighSessionsThreshold  = 10
	HighBgProcessThreshold = 5

	// Output capture
	OutputCaptureBufSize = 100 // channel buffer size
	OutputCaptureTimeout = 30  // seconds

	// Metrics buffer
	MetricsBufferSize = 1000

	// Session cleanup
	InactiveSessionTimeout = 60 // minutes

	// UUID validation pattern
	UUIDPattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
)
