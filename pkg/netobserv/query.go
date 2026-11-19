package netobserv

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const defaultTimeRangeSeconds int64 = 300

// PrepareQueryArguments converts MCP tool arguments into console-plugin query parameters.
// The plugin reads startTime/endTime (Unix seconds) and ignores timeRange.
func PrepareQueryArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return map[string]any{
			"endTime":   time.Now().Unix(),
			"startTime": time.Now().Unix() - defaultTimeRangeSeconds,
		}
	}

	prepared := make(map[string]any, len(arguments))
	for key, value := range arguments {
		if key == "timeRange" {
			continue
		}
		prepared[key] = value
	}

	startTime, hasStart := int64Arg(prepared["startTime"])
	endTime, hasEnd := int64Arg(prepared["endTime"])
	timeRange, hasTimeRange := int64Arg(arguments["timeRange"])
	now := time.Now().Unix()

	switch {
	case hasStart:
		prepared["startTime"] = startTime
		if hasEnd {
			prepared["endTime"] = endTime
		} else {
			prepared["endTime"] = now
		}
	case hasEnd:
		lookback := defaultTimeRangeSeconds
		if hasTimeRange {
			lookback = timeRange
		}
		prepared["endTime"] = endTime
		prepared["startTime"] = endTime - lookback
	default:
		lookback := defaultTimeRangeSeconds
		if hasTimeRange {
			lookback = timeRange
		}
		prepared["endTime"] = now
		prepared["startTime"] = now - lookback
	}

	return prepared
}

// ArgumentsToValues converts prepared MCP tool arguments into URL query values.
func ArgumentsToValues(arguments map[string]any) url.Values {
	values := url.Values{}
	if arguments == nil {
		return values
	}
	for key, value := range arguments {
		if value == nil {
			continue
		}
		if s := stringArg(value); s != "" {
			values.Set(key, s)
		}
	}
	return values
}

func int64Arg(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
		return 0, false
	case string:
		if v == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func stringArg(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}
