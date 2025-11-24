package config

import "context"

type configDirPathKey struct{}

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
