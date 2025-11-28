package kiali

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// getStringArgOrDefault returns the string argument value for the given key,
// or the provided default if the argument is absent or empty (after trimming).
// If key is "duration" or "rateInterval" and the provided value is a string,
// it is parsed via rateIntervalToSeconds and returned in seconds.
func getStringArgOrDefault(params api.ToolHandlerParams, key, defaultVal string) (string, error) {
	if raw, ok := params.GetArguments()[key]; ok {
		// Special handling for time-like keys that can be specified with suffixes
		if key == "duration" || key == "rateInterval" {
			if v, ok := raw.(string); ok {
				s := strings.TrimSpace(v)
				if s != "" {
					secs, err := rateIntervalToSeconds(s)
					if err != nil {
						return "", err
					}
					return strconv.FormatInt(secs, 10), nil
				}
				// if empty string, treat as missing and fall through to default handling
			}
		}

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
	// When missing or empty, for special keys also convert the default
	if key == "duration" || key == "rateInterval" {
		secs, err := rateIntervalToSeconds(defaultVal)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(secs, 10), nil
	}
	return defaultVal, nil
}

// rateIntervalToSeconds converts a rate interval string (e.g., "10m", "5h", "2d", "30s", "15")
// into its equivalent duration in seconds.
// Accepted suffixes:
//   - no suffix: seconds (e.g., "15" => 15)
//   - s: seconds (e.g., "30s" => 30)
//   - m: minutes (e.g., "10m" => 600)
//   - h: hours (e.g., "1h" => 3600)
//   - d: days (e.g., "2d" => 172800)
//
// Any other suffix returns an error.
func rateIntervalToSeconds(input string) (int64, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("rateInterval/duration is empty")
	}

	last := s[len(s)-1]
	var multiplier int64 = 1
	var numberPart string

	switch last {
	case 's':
		multiplier = 1
		numberPart = strings.TrimSpace(s[:len(s)-1])
	case 'm':
		multiplier = 60
		numberPart = strings.TrimSpace(s[:len(s)-1])
	case 'h':
		multiplier = 60 * 60
		numberPart = strings.TrimSpace(s[:len(s)-1])
	case 'd':
		multiplier = 24 * 60 * 60
		numberPart = strings.TrimSpace(s[:len(s)-1])
	default:
		// If last char is a digit, treat as seconds with no suffix
		if last >= '0' && last <= '9' {
			numberPart = s
		} else {
			return 0, fmt.Errorf("invalid rateInterval/duration suffix: %q", string(last))
		}
	}

	if numberPart == "" {
		return 0, fmt.Errorf("missing numeric value in rateInterval/duration")
	}

	// Only accept integer values
	n, err := strconv.ParseInt(numberPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value in rateInterval/duration: %w", err)
	}

	return n * multiplier, nil
}

// setQueryParam sets queryParams[key] from tool arguments (with default handling).
// It uses getStringArgOrDefault and wraps errors with a useful message depending on the key.
func setQueryParam(params api.ToolHandlerParams, queryParams map[string]string, key, defaultVal string) error {
	v, err := getStringArgOrDefault(params, key, defaultVal)
	if err != nil {
		switch key {
		case "duration", "rateInterval":
			return fmt.Errorf("invalid %s: %v, values must be in the format '10m', '5m', '1h', '2d' or seconds", key, err)
		default:
			return fmt.Errorf("invalid %s: %v", key, err)
		}
	}
	queryParams[key] = v
	return nil
}
