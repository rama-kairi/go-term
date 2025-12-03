// Package tracing provides OpenTelemetry-compatible tracing for command execution.
// This provides trace/span tracking for command execution with exportable data.
package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SpanContext represents a trace span context
type SpanContext struct {
	TraceID    string `json:"trace_id"`
	SpanID     string `json:"span_id"`
	ParentID   string `json:"parent_id,omitempty"`
	TraceFlags byte   `json:"trace_flags"`
}

// NewSpanContext creates a new span context with a new trace
func NewSpanContext() SpanContext {
	return SpanContext{
		TraceID:    generateTraceID(),
		SpanID:     generateSpanID(),
		TraceFlags: 1, // Sampled
	}
}

// NewChildSpanContext creates a child span context from a parent
func NewChildSpanContext(parent SpanContext) SpanContext {
	return SpanContext{
		TraceID:    parent.TraceID,
		SpanID:     generateSpanID(),
		ParentID:   parent.SpanID,
		TraceFlags: parent.TraceFlags,
	}
}

// SpanKind represents the type of span
type SpanKind int

const (
	SpanKindInternal SpanKind = iota
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// String returns the string representation of SpanKind
func (sk SpanKind) String() string {
	switch sk {
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "internal"
	}
}

// StatusCode represents the status of a span
type StatusCode int

const (
	StatusUnset StatusCode = iota
	StatusOK
	StatusError
)

// String returns the string representation of StatusCode
func (sc StatusCode) String() string {
	switch sc {
	case StatusOK:
		return "ok"
	case StatusError:
		return "error"
	default:
		return "unset"
	}
}

// Attribute represents a span attribute
type Attribute struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// Event represents a span event
type Event struct {
	Name       string      `json:"name"`
	Timestamp  time.Time   `json:"-"`
	Attributes []Attribute `json:"attributes,omitempty"`
}

// eventJSON is used for custom JSON marshaling
type eventJSON struct {
	Name       string      `json:"name"`
	Timestamp  string      `json:"timestamp"`
	Attributes []Attribute `json:"attributes,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for Event
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventJSON{
		Name:       e.Name,
		Timestamp:  e.Timestamp.Format(time.RFC3339),
		Attributes: e.Attributes,
	})
}

// Span represents a trace span
type Span struct {
	SpanContext SpanContext   `json:"-"`
	Name        string        `json:"-"`
	Kind        SpanKind      `json:"-"`
	StartTime   time.Time     `json:"-"`
	EndTime     time.Time     `json:"-"`
	Duration    time.Duration `json:"-"`
	Status      StatusCode    `json:"-"`
	StatusMsg   string        `json:"-"`
	Attributes  []Attribute   `json:"-"`
	Events      []Event       `json:"-"`

	// Internal state
	isEnded bool
	mutex   sync.Mutex
}

// spanJSON is used for custom JSON marshaling
type spanJSON struct {
	SpanContext SpanContext `json:"span_context"`
	Name        string      `json:"name"`
	Kind        SpanKind    `json:"kind"`
	StartTime   string      `json:"start_time"`
	EndTime     string      `json:"end_time"`
	Duration    int64       `json:"duration"`
	Status      StatusCode  `json:"status"`
	StatusMsg   string      `json:"status_message,omitempty"`
	Attributes  []Attribute `json:"attributes,omitempty"`
	Events      []Event     `json:"events,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for Span
func (s *Span) MarshalJSON() ([]byte, error) {
	return json.Marshal(spanJSON{
		SpanContext: s.SpanContext,
		Name:        s.Name,
		Kind:        s.Kind,
		StartTime:   s.StartTime.Format(time.RFC3339),
		EndTime:     s.EndTime.Format(time.RFC3339),
		Duration:    int64(s.Duration),
		Status:      s.Status,
		StatusMsg:   s.StatusMsg,
		Attributes:  s.Attributes,
		Events:      s.Events,
	})
}

// NewSpan creates a new span
func NewSpan(name string, kind SpanKind) *Span {
	return &Span{
		SpanContext: NewSpanContext(),
		Name:        name,
		Kind:        kind,
		StartTime:   time.Now(),
		Status:      StatusUnset,
		Attributes:  make([]Attribute, 0),
		Events:      make([]Event, 0),
	}
}

// NewChildSpan creates a new child span from a parent context
func NewChildSpan(ctx context.Context, name string, kind SpanKind) *Span {
	span := &Span{
		SpanContext: NewSpanContext(),
		Name:        name,
		Kind:        kind,
		StartTime:   time.Now(),
		Status:      StatusUnset,
		Attributes:  make([]Attribute, 0),
		Events:      make([]Event, 0),
	}

	// Check for parent span in context
	if parent := SpanFromContext(ctx); parent != nil {
		span.SpanContext = NewChildSpanContext(parent.SpanContext)
	}

	return span
}

// SetAttribute sets an attribute on the span
func (s *Span) SetAttribute(key string, value interface{}) *Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Update existing or add new
	for i, attr := range s.Attributes {
		if attr.Key == key {
			s.Attributes[i].Value = value
			return s
		}
	}

	s.Attributes = append(s.Attributes, Attribute{Key: key, Value: value})
	return s
}

// SetAttributes sets multiple attributes on the span
func (s *Span) SetAttributes(attrs map[string]interface{}) *Span {
	for key, value := range attrs {
		s.SetAttribute(key, value)
	}
	return s
}

// AddEvent adds an event to the span
func (s *Span) AddEvent(name string, attrs ...Attribute) *Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.Events = append(s.Events, Event{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
	return s
}

// SetStatus sets the status of the span
func (s *Span) SetStatus(code StatusCode, message string) *Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.Status = code
	s.StatusMsg = message
	return s
}

// End ends the span
func (s *Span) End() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isEnded {
		return
	}

	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
	s.isEnded = true
}

// TraceID returns the trace ID
func (s *Span) TraceID() string {
	return s.SpanContext.TraceID
}

// SpanID returns the span ID
func (s *Span) SpanID() string {
	return s.SpanContext.SpanID
}

// --- Context handling ---

type spanContextKey struct{}

// ContextWithSpan returns a new context with the span attached
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

// SpanFromContext returns the span from the context, or nil if not found
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return span
	}
	return nil
}

// --- Tracer ---

// Tracer provides span creation and trace management
type Tracer struct {
	serviceName string
	spans       []*Span
	mutex       sync.RWMutex
	exporters   []SpanExporter
	maxSpans    int
}

// SpanExporter exports spans to an external system
type SpanExporter interface {
	Export(spans []*Span) error
	Shutdown() error
}

// NewTracer creates a new tracer
func NewTracer(serviceName string) *Tracer {
	return &Tracer{
		serviceName: serviceName,
		spans:       make([]*Span, 0),
		exporters:   make([]SpanExporter, 0),
		maxSpans:    1000, // Keep max 1000 spans in memory
	}
}

// StartSpan starts a new span
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := NewChildSpan(ctx, name, SpanKindInternal)
	span.SetAttribute("service.name", t.serviceName)

	t.addSpan(span)

	return ContextWithSpan(ctx, span), span
}

// StartSpanWithKind starts a new span with a specific kind
func (t *Tracer) StartSpanWithKind(ctx context.Context, name string, kind SpanKind) (context.Context, *Span) {
	span := NewChildSpan(ctx, name, kind)
	span.SetAttribute("service.name", t.serviceName)

	t.addSpan(span)

	return ContextWithSpan(ctx, span), span
}

// addSpan adds a span to the tracer's collection
func (t *Tracer) addSpan(span *Span) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.spans = append(t.spans, span)

	// Trim old spans if we exceed the limit
	if len(t.spans) > t.maxSpans {
		t.spans = t.spans[len(t.spans)-t.maxSpans:]
	}
}

// GetSpans returns all collected spans
func (t *Tracer) GetSpans() []*Span {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	result := make([]*Span, len(t.spans))
	copy(result, t.spans)
	return result
}

// GetRecentSpans returns the most recent spans
func (t *Tracer) GetRecentSpans(limit int) []*Span {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if limit > len(t.spans) {
		limit = len(t.spans)
	}

	result := make([]*Span, limit)
	copy(result, t.spans[len(t.spans)-limit:])
	return result
}

// AddExporter adds a span exporter
func (t *Tracer) AddExporter(exporter SpanExporter) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.exporters = append(t.exporters, exporter)
}

// Export exports all spans to registered exporters
func (t *Tracer) Export() error {
	t.mutex.RLock()
	spans := make([]*Span, len(t.spans))
	copy(spans, t.spans)
	exporters := make([]SpanExporter, len(t.exporters))
	copy(exporters, t.exporters)
	t.mutex.RUnlock()

	var lastErr error
	for _, exporter := range exporters {
		if err := exporter.Export(spans); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Shutdown shuts down the tracer and all exporters
func (t *Tracer) Shutdown() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	var lastErr error
	for _, exporter := range t.exporters {
		if err := exporter.Shutdown(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// ClearSpans clears all collected spans
func (t *Tracer) ClearSpans() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.spans = make([]*Span, 0)
}

// --- Helper functions ---

// generateTraceID generates a 32-character hex trace ID
func generateTraceID() string {
	id := uuid.New()
	return fmt.Sprintf("%032x", id[:])[:32]
}

// generateSpanID generates a 16-character hex span ID
func generateSpanID() string {
	id := uuid.New()
	return fmt.Sprintf("%016x", id[:8])[:16]
}

// --- Standard attributes for command execution ---

const (
	// Service attributes
	AttrServiceName    = "service.name"
	AttrServiceVersion = "service.version"

	// Command execution attributes
	AttrCommand      = "command.text"
	AttrCommandType  = "command.type"
	AttrSessionID    = "session.id"
	AttrSessionName  = "session.name"
	AttrProjectID    = "project.id"
	AttrWorkingDir   = "working.directory"
	AttrExitCode     = "exit.code"
	AttrOutputSize   = "output.size"
	AttrIsBackground = "is.background"

	// Error attributes
	AttrErrorType    = "error.type"
	AttrErrorMessage = "error.message"
)
