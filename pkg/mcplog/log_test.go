package mcplog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LoggingSuite struct {
	suite.Suite
}

func (s *LoggingSuite) TestSanitizeMessage() {
	s.Run("redacts passwords", func() {
		msg := `{"password": "secret123"}` // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "secret123")
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts bearer tokens", func() {
		msg := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts basic auth", func() {
		msg := "Authorization: Basic dXNlcjpwYXNzd29yZA==" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "dXNlcjpwYXNzd29yZA==") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts token fields", func() {
		msg := `{"token": "abc123def456"}` // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "abc123def456")
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts secret fields", func() {
		msg := `{"secret": "my-secret-value"}`
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "my-secret-value")
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts api_key fields", func() {
		msg := `{"api_key": "12345abcde", "api-key": "67890fghij"}` // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "12345abcde")
		s.NotContains(sanitized, "67890fghij")
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts AWS access keys", func() {
		msg := "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "AKIAIOSFODNN7EXAMPLE") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts AWS secret access keys", func() {
		msg := "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts GitHub tokens", func() {
		msg := "token: ghp_1234567890abcdefghijklmnopqrstuv1234" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "ghp_1234567890abcdefghijklmnopqrstuv1234") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts GitLab tokens", func() {
		msg := "GITLAB_TOKEN=glpat-abcdefghij1234567890" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "glpat-abcdefghij1234567890") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts GCP API keys", func() {
		msg := "apiKey: AIzaSyDaGmWKa4JsXZ-HjGw7ISLn_3namBGewQe" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "AIzaSyDaGmWKa4JsXZ-HjGw7ISLn_3namBGewQe") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts OpenAI API keys", func() {
		msg := "OPENAI_API_KEY=sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuv" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuv") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts Anthropic API keys", func() {
		msg := "key: sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts JWT tokens", func() {
		msg := "token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c" // notsecret
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c") // notsecret
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts SSH private keys", func() {
		msg := "key: -----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA"
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "-----BEGIN RSA PRIVATE KEY-----")
		s.Contains(sanitized, "[REDACTED]")
	})

	s.Run("redacts PostgreSQL connection strings", func() {
		msg := "DB_URL=postgres://user:mypassword@localhost:5432/db"
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "mypassword")
		s.Contains(sanitized, "postgres://user:[REDACTED]@")
	})

	s.Run("redacts MySQL connection strings", func() {
		msg := "mysql://admin:secretpass@db.example.com:3306/prod"
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "secretpass")
		s.Contains(sanitized, "mysql://admin:[REDACTED]@")
	})

	s.Run("redacts MongoDB connection strings", func() {
		msg := "MONGO_URI=mongodb+srv://dbuser:dbpass123@cluster.mongodb.net/mydb"
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "dbpass123")
		s.Contains(sanitized, "mongodb+srv://dbuser:[REDACTED]@")
	})

	s.Run("preserves non-sensitive data", func() {
		msg := `{"namespace": "default", "pod": "nginx"}`
		sanitized := sanitizeMessage(msg)
		s.Contains(sanitized, "default")
		s.Contains(sanitized, "nginx")
	})

	s.Run("handles multiple sensitive fields", func() {
		msg := `{"password": "pass123", "token": "tok456", "namespace": "default"}`
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "pass123")
		s.NotContains(sanitized, "tok456")
		s.Contains(sanitized, "[REDACTED]")
		s.Contains(sanitized, "default")
	})

	s.Run("handles mixed secret types", func() {
		msg := `Failed to connect: {"password": "dbpass", "token": "ghp_1234567890abcdefghijklmnopqrstuv1234", "api_key": "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuv"}`
		sanitized := sanitizeMessage(msg)
		s.NotContains(sanitized, "dbpass")
		s.NotContains(sanitized, "ghp_1234567890abcdefghijklmnopqrstuv1234")
		s.NotContains(sanitized, "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuv")
		s.Contains(sanitized, "Failed to connect")
		s.Contains(sanitized, "[REDACTED]")
	})
}

func (s *LoggingSuite) TestSendMCPLogWithoutSession() {
	s.Run("does not panic without session in context", func() {
		ctx := context.Background()
		s.NotPanics(func() {
			SendMCPLog(ctx, LevelInfo, "test message")
		})
	})

	s.Run("handles all log levels without session", func() {
		ctx := context.Background()
		levels := []Level{LevelDebug, LevelInfo, LevelNotice, LevelWarning, LevelError, LevelCritical, LevelAlert, LevelEmergency}
		for _, level := range levels {
			s.NotPanics(func() {
				SendMCPLog(ctx, level, "test message for level "+level.String())
			})
		}
	})

	s.Run("sanitizes message even without session", func() {
		ctx := context.Background()
		// This should not panic and should sanitize the message in server logs
		s.NotPanics(func() {
			SendMCPLog(ctx, LevelError, "Failed with password: secret123")
		})
	})
}

func TestLogging(t *testing.T) {
	suite.Run(t, new(LoggingSuite))
}
