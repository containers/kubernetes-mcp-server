package config

import "context"

type configDirPathKey struct{}
type requireTLSKey struct{}

func withConfigDirPath(ctx context.Context, dirPath string) context.Context {
	return context.WithValue(ctx, configDirPathKey{}, dirPath)
}

func ConfigDirPathFromContext(ctx context.Context) string {
	val := ctx.Value(configDirPathKey{})

	if val == nil {
		return ""
	}

	if strVal, ok := val.(string); ok {
		return strVal
	}

	return ""
}

func withRequireTLS(ctx context.Context, requireTLS bool) context.Context {
	return context.WithValue(ctx, requireTLSKey{}, requireTLS)
}

func RequireTLSFromContext(ctx context.Context) bool {
	val := ctx.Value(requireTLSKey{})
	if val == nil {
		return false
	}
	if boolVal, ok := val.(bool); ok {
		return boolVal
	}
	return false
}
