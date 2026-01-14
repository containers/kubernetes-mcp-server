package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// getStringArgOrDefault returns the string argument value for the given key,
// or the provided default if the argument is absent or empty (after trimming).
func getStringArgOrDefault(params api.ToolHandlerParams, key, defaultVal string) (string, error) {
	if raw, ok := params.GetArguments()[key]; ok {
		// Special handling for time-like keys that can be specified with suffixes

		switch v := raw.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s != "" {
				return s, nil
			}
		case float64:
			// JSON numbers decode to float64 in maps; use -1 precision to trim trailing zeros
			return strconv.FormatFloat(v, 'f', -1, 64), nil
		case int:
			return strconv.Itoa(v), nil
		case int64:
			return strconv.FormatInt(v, 10), nil
		case json.Number:
			return v.String(), nil
		default:
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" {
				return s, nil
			}
		}
	}
	return defaultVal, nil
}

// setQueryParam sets queryParams[key] from tool arguments (with default handling).
// It uses getStringArgOrDefault and wraps errors with a useful message.
func setQueryParam(params api.ToolHandlerParams, queryParams map[string]string, key, defaultVal string) error {
	v, err := getStringArgOrDefault(params, key, defaultVal)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", key, err)
	}
	queryParams[key] = v
	return nil
}
