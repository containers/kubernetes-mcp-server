package klogutil

import (
	"k8s.io/klog/v2"
)

// Attr is a single, complete key/value log attribute. Because every constructor
// returns a complete pair, attributes compose through the variadic logr/klog
// logging API without the "even number of varargs" footgun: keys and values
// cannot get out of sync.
type Attr struct {
	K string
	V any
}

// Err returns an Attr carrying the error message under the OpenTelemetry
// semantic-convention key "exception.message". It centralizes that key so there
// is a single source of truth across every error log site. A nil error yields an
// empty message rather than panicking.
func Err(err error) Attr {
	if err == nil {
		return Attr{"exception.message", ""}
	}
	return Attr{"exception.message", err.Error()}
}

// Field returns an Attr for an arbitrary key/value pair.
func Field(k string, v any) Attr { return Attr{k, v} }

// toKV flattens a slice of Attr into the alternating key/value slice expected by
// the logr/klog variadic logging API.
func toKV(attrs []Attr) []any {
	kv := make([]any, 0, len(attrs)*2)
	for _, a := range attrs {
		kv = append(kv, a.K, a.V)
	}
	return kv
}

// LogInfo logs msg with the provided attributes on the given logger. Pass a
// V-leveled logger (e.g. logger.V(4)) to preserve verbosity gating.
func LogInfo(logger klog.Logger, msg string, attrs ...Attr) {
	logger.Info(msg, toKV(attrs)...)
}

// LogWarn logs msg with a log.severity=WARN tag and the provided attributes.
// This is done because the klog.Logger interface has no native Warn method. Pass
// a V-leveled logger (e.g. logger.V(1)) to preserve verbosity gating.
func LogWarn(logger klog.Logger, msg string, attrs ...Attr) {
	logger.Info(msg, append([]any{"log.severity", "WARN"}, toKV(attrs)...)...)
}
