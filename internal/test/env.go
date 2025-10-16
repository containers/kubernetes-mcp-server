package test

import (
	"os"
	"strings"
)

func RestoreEnv(originalEnv []string) {
	os.Clearenv()
	for _, env := range originalEnv {
		if key, value, found := strings.Cut(env, "="); found {
			_ = os.Setenv(key, value)
		}
	}
}
