package netobserv

import (
	"fmt"
	"net/url"
	"strconv"
)

// ArgumentsToValues converts MCP tool arguments into URL query values for the console plugin API.
func ArgumentsToValues(arguments map[string]any) url.Values {
	values := url.Values{}
	if arguments == nil {
		return values
	}
	for key, value := range arguments {
		if value == nil {
			continue
		}
		switch key {
		case "match":
			if s, ok := stringArg(value); ok && s != "" {
				values.Add("match[]", "{"+s+"}")
			}
		default:
			if s, ok := stringArg(value); ok && s != "" {
				values.Set(key, s)
			}
		}
	}
	return values
}

func stringArg(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	default:
		return fmt.Sprint(v), true
	}
}
