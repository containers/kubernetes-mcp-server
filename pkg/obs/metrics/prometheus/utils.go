package prometheus

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

func ParseTimestamp(timestamp string) (time.Time, error) {
	// Handle NOW keyword (case-insensitive)
	if strings.EqualFold(timestamp, "NOW") {
		return time.Now(), nil
	}

	// Handle relative time expressions like NOW-5m, NOW+1h
	upper := strings.ToUpper(timestamp)
	if strings.HasPrefix(upper, "NOW") && len(timestamp) > 3 {
		rest := timestamp[3:]

		// Check if the next character is + or -
		if rest != "" && (rest[0] == '+' || rest[0] == '-') {
			isNegative := rest[0] == '-'
			durationStr := rest[1:]

			// Parse the duration using Prometheus model
			duration, err := model.ParseDuration(durationStr)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid duration in relative time expression: %s", err.Error())
			}

			offset := time.Duration(duration)
			if isNegative {
				return time.Now().Add(-offset), nil
			}
			return time.Now().Add(offset), nil
		}
	}

	// Try parsing as RFC3339 first
	if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return t, nil
	}

	// Try parsing as Unix timestamp
	if unixTime, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		return time.Unix(unixTime, 0), nil
	}

	return time.Time{}, fmt.Errorf("timestamp must be RFC3339 format, Unix timestamp, NOW, or relative time (NOW±duration)")
}
