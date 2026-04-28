// Source: https://github.com/jaegertracing/jaeger-operator/blob/88fd9c6ef1d254dbf7946d2cfaf9d42acecdea4c/pkg/util/dns_name.go (ASL 2.0 license)
// Must follow same algorithm as service name generation in tempo operator:
// https://github.com/grafana/tempo-operator/blob/9c3430969265e23b2e9fc3103ab608624195e15e/internal/manifests/naming/naming.go#L13
// https://github.com/grafana/tempo-operator/blob/9c3430969265e23b2e9fc3103ab608624195e15e/internal/manifests/naming/sanitize.go
package discovery

import (
	"regexp"
	"strings"
)

var (
	regex = regexp.MustCompile(`[a-z0-9]`)
)

// DNSName returns a dns-safe string for the given name.
// Any char that is not [a-z0-9] is replaced by "-" or "a".
// Replacement character "a" is used only at the beginning or at the end of the name.
// The function does not change length of the string.
func DNSName(name string) string {
	nameLower := []rune(strings.ToLower(name))
	var d []rune

	for i, x := range nameLower {
		if regex.MatchString(string(x)) {
			d = append(d, x)
		} else {
			if i == 0 || i == len(nameLower)-1 {
				d = append(d, 'a')
			} else {
				d = append(d, '-')
			}
		}
	}

	return string(d)
}
