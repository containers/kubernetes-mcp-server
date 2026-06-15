package klogutil

import (
	"context"

	"k8s.io/klog/v2"
)

func Warn(ctx context.Context, msg string, kv ...any) {
	klog.FromContext(ctx).Info(msg, append([]any{"log.severity", "WARN"}, kv...)...)
}
