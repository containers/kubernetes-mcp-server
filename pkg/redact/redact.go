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
	pemBegin,
	"ssh-rsa ", "ssh-ed25519 ", "ssh-dss ",
	"password=", "passwd=", "token=",
}

// pemBegin opens a PEM block; the lines up to pemEnd are the key material.
const (
	pemBegin = "-----begin"
	pemEnd   = "-----end"
)

// Redacted replaces everything after a secret marker on a line.
const Redacted = "<REDACTED>"

// Secrets redacts the remainder of every line that contains a secret marker.
// The body of a PEM block is replaced by a single Redacted line up to its
// end marker, or to the end of the text when the block is unterminated.
func Secrets(text string) string {
	var out []string
	inPEM := false
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		if inPEM {
			if strings.Contains(lower, pemEnd) {
				inPEM = false
				out = append(out, line)
			}
			continue
		}
		for _, pattern := range secretPatterns {
			if idx := strings.Index(lower, pattern); idx >= 0 {
				rest := lower[idx+len(pattern):]
				line = line[:idx+len(pattern)] + Redacted
				// A PEM block that begins and ends on the same line (e.g. a
				// JSON-escaped certificate) is fully covered by the single
				// line redaction above; entering multi-line mode would swallow
				// every following line.
				if pattern == pemBegin && !strings.Contains(rest, pemEnd) {
					inPEM = true
					line += "\n" + Redacted
				}
				break
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
