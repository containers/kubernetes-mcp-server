// Package redact removes secrets from free-form text such as container logs
// before it is handed to a model.
package redact

import "strings"

// secretPatterns are case-insensitive markers after which the rest of the
// line is replaced. They cover bearer tokens, basic-auth flags, PEM blocks,
// SSH keys and key=value credentials.
var secretPatterns = []string{
	"bearer ",
	"authorization:",
	"authorization=",
	"-u ", "--user ",
	"-----begin",
	"ssh-rsa ", "ssh-ed25519 ", "ssh-dss ",
	"password=", "passwd=", "token=",
}

// Redacted replaces everything after a secret marker on a line.
const Redacted = "<REDACTED>"

// Secrets redacts the remainder of every line that contains a secret marker.
func Secrets(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, pattern := range secretPatterns {
			if idx := strings.Index(lower, pattern); idx >= 0 {
				lines[i] = line[:idx+len(pattern)] + Redacted
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}
