// Package sanitize redacts secret-shaped values from diagnostic text and logs.
package sanitize

import (
	"regexp"
	"strings"
)

var sensitivePatterns = []*regexp.Regexp{
	// Generic JSON/YAML fields.
	regexp.MustCompile(`("password"\s*:\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`("token"\s*:\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`("secret"\s*:\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`("api[_-]?key"\s*:\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`("access[_-]?key"\s*:\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`("client[_-]?secret"\s*:\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`("private[_-]?key"\s*:\s*)"(?:\\.|[^"\\])*"`),
	// Authorization headers.
	regexp.MustCompile(`(Bearer\s+)[A-Za-z0-9\-._~+/]+=*`),
	regexp.MustCompile(`(Basic\s+)[A-Za-z0-9+/]+=*`),
	// AWS credentials.
	regexp.MustCompile(`(AKIA[0-9A-Z]{16})`),
	regexp.MustCompile(`(aws_secret_access_key\s*=\s*)([A-Za-z0-9/+=]{40})`),
	regexp.MustCompile(`(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}`),
	// GitHub tokens.
	regexp.MustCompile(`(ghp_[a-zA-Z0-9]{36})`),
	regexp.MustCompile(`(github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})`),
	// GitLab tokens.
	regexp.MustCompile(`(glpat-[a-zA-Z0-9\-_]{20})`),
	// GCP.
	regexp.MustCompile(`(AIza[0-9A-Za-z\-_]{35})`),
	// Azure.
	regexp.MustCompile(`(AccountKey=[A-Za-z0-9+/]{88}==)`),
	// OpenAI / Anthropic.
	regexp.MustCompile(`(sk-proj-[a-zA-Z0-9]{48})`),
	regexp.MustCompile(`(sk-ant-api03-[a-zA-Z0-9\-_]{95})`),
	// JWT tokens.
	regexp.MustCompile(`(eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+)`),
	// Private keys.
	regexp.MustCompile(`(-----BEGIN[A-Z ]+PRIVATE KEY-----)`),
	regexp.MustCompile(`(-----BEGIN RSA PRIVATE KEY-----)`),
	regexp.MustCompile(`(-----BEGIN EC PRIVATE KEY-----)`),
	regexp.MustCompile(`(-----BEGIN OPENSSH PRIVATE KEY-----)`),
	regexp.MustCompile(`(-----BEGIN PGP PRIVATE KEY BLOCK-----)`),
	// Database connection strings.
	regexp.MustCompile(`(postgres://[^:]+:)([^@]+)(@)`),
	regexp.MustCompile(`(mysql://[^:]+:)([^@]+)(@)`),
	regexp.MustCompile(`(mongodb(\+srv)?://[^:]+:)([^@]+)(@)`),
}

var (
	pemHeaderPattern        = regexp.MustCompile(`-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY|PGP PRIVATE KEY BLOCK)-----`)
	logSecretPattern        = regexp.MustCompile(`(?i)(\b"?(?:password|passwd|token|secret|credentials?|api[_-]?key|apikey|aws_secret_access_key|auth)"?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'[^']*'|[^\s,;}\]]+)`)
	yamlSecretBlockPattern  = regexp.MustCompile(`(?i)^(\s*"?(?:password|passwd|token|secret|credentials?|api[_-]?key|apikey|aws_secret_access_key|auth)"?\s*:\s*)[|>][0-9+-]*\s*(?:#.*)?$`)
	authorizationPattern    = regexp.MustCompile(`(?i)(\bauthorization\s*[:=]\s*)(?:(?:bearer|basic)\s+)?[^\s,;}]+`)
	credentialSchemePattern = regexp.MustCompile(`(?i)(\b(?:bearer|basic)\s+)[^\s,;}]+`)
	sshKeyPattern           = regexp.MustCompile(`(?i)(\b(?:ssh-rsa|ssh-ed25519|ssh-dss)\s+)[^\s]+`)
)

// Text redacts known secret-shaped substrings from text. It is best-effort;
// callers handling structured credentials should avoid logging them at all.
func Text(text string) string {
	// JSON/YAML field patterns (indices 0-6) preserve the field name.
	for i := 0; i < 7 && i < len(sensitivePatterns); i++ {
		text = sensitivePatterns[i].ReplaceAllString(text, `$1"[REDACTED]"`)
	}

	// Authorization headers (indices 7-8) preserve the header type.
	for i := 7; i < 9 && i < len(sensitivePatterns); i++ {
		text = sensitivePatterns[i].ReplaceAllString(text, `$1[REDACTED]`)
	}

	// Database connection strings (indices 25-27) preserve the URL structure.
	text = sensitivePatterns[25].ReplaceAllString(text, `$1[REDACTED]$3`)
	text = sensitivePatterns[26].ReplaceAllString(text, `$1[REDACTED]$3`)
	text = sensitivePatterns[27].ReplaceAllString(text, `$1[REDACTED]$4`)

	// All remaining patterns redact the complete match.
	for i := 9; i < len(sensitivePatterns); i++ {
		if i >= 25 && i <= 27 {
			continue
		}
		text = sensitivePatterns[i].ReplaceAllString(text, `[REDACTED]`)
	}
	return text
}

// Log redacts line-oriented command output. The boolean reports whether a PEM
// block was omitted, allowing callers to mark the returned evidence truncated.
func Log(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	pemLabel := ""
	yamlSecretIndent := -1
	redactedBlock := false

	for _, rawLine := range lines {
		if pemLabel != "" {
			if strings.Contains(rawLine, "-----END "+pemLabel+"-----") {
				pemLabel = ""
			}
			continue
		}
		if yamlSecretIndent >= 0 {
			if strings.TrimSpace(rawLine) == "" || len(rawLine)-len(strings.TrimLeft(rawLine, " ")) > yamlSecretIndent {
				continue
			}
			yamlSecretIndent = -1
		}

		if match := pemHeaderPattern.FindStringSubmatchIndex(rawLine); match != nil {
			result = append(result, Text(rawLine[:match[0]])+"[REDACTED PEM BLOCK]")
			pemLabel = rawLine[match[2]:match[3]]
			redactedBlock = true
			continue
		}
		if yamlSecretBlockPattern.MatchString(rawLine) {
			result = append(result, yamlSecretBlockPattern.ReplaceAllString(rawLine, `$1[REDACTED]`))
			yamlSecretIndent = len(rawLine) - len(strings.TrimLeft(rawLine, " "))
			redactedBlock = true
			continue
		}

		line := Text(rawLine)
		line = authorizationPattern.ReplaceAllString(line, `$1[REDACTED]`)
		line = credentialSchemePattern.ReplaceAllString(line, `$1[REDACTED]`)
		line = logSecretPattern.ReplaceAllString(line, `$1[REDACTED]`)
		line = sshKeyPattern.ReplaceAllString(line, `$1[REDACTED]`)
		result = append(result, line)
	}
	return strings.Join(result, "\n"), redactedBlock
}
