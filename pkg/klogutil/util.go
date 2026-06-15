package klogutil

import (
	"context"

	"k8s.io/klog/v2"
)

// Warn obtains a klog logger from context and logs the provided message + kvs
// with a log.severity=WARN. This is done as klog.Logger interface does not have
// a native Warn method
func Warn(ctx context.Context, msg string, kv ...any) {
	klog.FromContext(ctx).Info(msg, append([]any{"log.severity", "WARN"}, kv...)...)
}

// WarnLogger logs the provided message + kvs with a log.severity=WARN on the
// provided logger. This is done as klog.Logger interface does not have a native
// Warn method
func WarnLogger(logger klog.Logger, msg string, kv ...any) {
	logger.Info(msg, append([]any{"log.severity", "WARN"}, kv...)...)
}
