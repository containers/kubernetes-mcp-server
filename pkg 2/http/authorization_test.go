package http

import (
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	// https://jwt.io/#token=eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrdWJlcm5ldGVzLW1jcC1zZXJ2ZXIiXSwiZXhwIjoyNTM0MDIyOTcxOTksImlhdCI6MCwiaXNzIjoiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJqdGkiOiI5OTIyMmQ1Ni0zNDBlLTRlYjYtODU4OC0yNjE0MTFmMzVkMjYiLCJrdWJlcm5ldGVzLmlvIjp7Im5hbWVzcGFjZSI6ImRlZmF1bHQiLCJzZXJ2aWNlYWNjb3VudCI6eyJuYW1lIjoiZGVmYXVsdCIsInVpZCI6ImVhY2I2YWQyLTgwYjctNDE3OS04NDNkLTkyZWIxZTZiYmJhNiJ9fSwibmJmIjowLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpkZWZhdWx0In0.0363P6xGmWpU-O9TAVkcOd95lPXxhI-_k5NKbHGNQeL--B8XMAz2vC8hpKnyC6rKOGifRTSR2XNHx_5fjd7lEA // notsecret
	tokenBasicNotExpired = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrdWJlcm5ldGVzLW1jcC1zZXJ2ZXIiXSwiZXhwIjoyNTM0MDIyOTcxOTksImlhdCI6MCwiaXNzIjoiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJqdGkiOiI5OTIyMmQ1Ni0zNDBlLTRlYjYtODU4OC0yNjE0MTFmMzVkMjYiLCJrdWJlcm5ldGVzLmlvIjp7Im5hbWVzcGFjZSI6ImRlZmF1bHQiLCJzZXJ2aWNlYWNjb3VudCI6eyJuYW1lIjoiZGVmYXVsdCIsInVpZCI6ImVhY2I2YWQyLTgwYjctNDE3OS04NDNkLTkyZWIxZTZiYmJhNiJ9fSwibmJmIjowLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpkZWZhdWx0In0.0363P6xGmWpU-O9TAVkcOd95lPXxhI-_k5NKbHGNQeL--B8XMAz2vC8hpKnyC6rKOGifRTSR2XNHx_5fjd7lEA" // notsecret
	// https://jwt.io/#token=eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrdWJlcm5ldGVzLW1jcC1zZXJ2ZXIiXSwiZXhwIjoxLCJpYXQiOjAsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOTkyMjJkNTYtMzQwZS00ZWI2LTg1ODgtMjYxNDExZjM1ZDI2Iiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImRlZmF1bHQiLCJ1aWQiOiJlYWNiNmFkMi04MGI3LTQxNzktODQzZC05MmViMWU2YmJiYTYifX0sIm5iZiI6MCwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGVmYXVsdCJ9.USsuGLsB_7MwG9i0__cFkVVZa0djtmQpc8Vwi56GrapAgVAcyTfmae3s83XMDP5AwcFnxhYxLCfiZWRJri6GTA // notsecret
	tokenBasicExpired = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrdWJlcm5ldGVzLW1jcC1zZXJ2ZXIiXSwiZXhwIjoxLCJpYXQiOjAsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOTkyMjJkNTYtMzQwZS00ZWI2LTg1ODgtMjYxNDExZjM1ZDI2Iiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImRlZmF1bHQiLCJ1aWQiOiJlYWNiNmFkMi04MGI3LTQxNzktODQzZC05MmViMWU2YmJiYTYifX0sIm5iZiI6MCwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGVmYXVsdCJ9.USsuGLsB_7MwG9i0__cFkVVZa0djtmQpc8Vwi56GrapAgVAcyTfmae3s83XMDP5AwcFnxhYxLCfiZWRJri6GTA" // notsecret
	// https://jwt.io/#token=eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrdWJlcm5ldGVzLW1jcC1zZXJ2ZXIiXSwiZXhwIjoyNTM0MDIyOTcxOTksImlhdCI6MCwiaXNzIjoiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJqdGkiOiI5OTIyMmQ1Ni0zNDBlLTRlYjYtODU4OC0yNjE0MTFmMzVkMjYiLCJrdWJlcm5ldGVzLmlvIjp7Im5hbWVzcGFjZSI6ImRlZmF1bHQiLCJzZXJ2aWNlYWNjb3VudCI6eyJuYW1lIjoiZGVmYXVsdCIsInVpZCI6ImVhY2I2YWQyLTgwYjctNDE3OS04NDNkLTkyZWIxZTZiYmJhNiJ9fSwibmJmIjowLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpkZWZhdWx0Iiwic2NvcGUiOiJyZWFkIHdyaXRlIn0.vl5se9BuxoVDhvR7M5wGfkLoyMSYUiORMZVxl0CQ7jw3x53mZfGEkU_kkIVIl9Ui371qCCVVxdvuZPcAgbM6pQ // notsecret
	tokenMultipleAudienceNotExpired = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrdWJlcm5ldGVzLW1jcC1zZXJ2ZXIiXSwiZXhwIjoyNTM0MDIyOTcxOTksImlhdCI6MCwiaXNzIjoiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJqdGkiOiI5OTIyMmQ1Ni0zNDBlLTRlYjYtODU4OC0yNjE0MTFmMzVkMjYiLCJrdWJlcm5ldGVzLmlvIjp7Im5hbWVzcGFjZSI6ImRlZmF1bHQiLCJzZXJ2aWNlYWNjb3VudCI6eyJuYW1lIjoiZGVmYXVsdCIsInVpZCI6ImVhY2I2YWQyLTgwYjctNDE3OS04NDNkLTkyZWIxZTZiYmJhNiJ9fSwibmJmIjowLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpkZWZhdWx0Iiwic2NvcGUiOiJyZWFkIHdyaXRlIn0.vl5se9BuxoVDhvR7M5wGfkLoyMSYUiORMZVxl0CQ7jw3x53mZfGEkU_kkIVIl9Ui371qCCVVxdvuZPcAgbM6pQ" // notsecret
)

func TestParseJWTClaimsPayloadValid(t *testing.T) {
	basicClaims, err := ParseJWTClaims(tokenBasicNotExpired)
	t.Run("Is parseable", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if basicClaims == nil {
			t.Fatal("expected claims, got nil")
		}
	})
	t.Run("Parses issuer", func(t *testing.T) {
		if basicClaims.Issuer != "https://kubernetes.default.svc.cluster.local" {
			t.Errorf("expected issuer 'https://kubernetes.default.svc.cluster.local', got %s", basicClaims.Issuer)
		}
	})
	t.Run("Parses audience", func(t *testing.T) {
		expectedAudiences := []string{"https://kubernetes.default.svc.cluster.local", "kubernetes-mcp-server"}
		for _, expected := range expectedAudiences {
			if !basicClaims.Audience.Contains(expected) {
				t.Errorf("expected audience to contain %s", expected)
			}
		}
	})
	t.Run("Parses expiration", func(t *testing.T) {
		if *basicClaims.Expiry != jwt.NumericDate(253402297199) {
			t.Errorf("expected expiration 253402297199, got %d", basicClaims.Expiry)
		}
	})
	t.Run("Parses scope", func(t *testing.T) {
		scopeClaims, err := ParseJWTClaims(tokenMultipleAudienceNotExpired)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if scopeClaims == nil {
			t.Fatal("expected claims, got nil")
		}

		scopes := scopeClaims.GetScopes()

		expectedScopes := []string{"read", "write"}
		if len(scopes) != len(expectedScopes) {
			t.Errorf("expected %d scopes, got %d", len(expectedScopes), len(scopes))
		}
		for i, expectedScope := range expectedScopes {
			if scopes[i] != expectedScope {
				t.Errorf("expected scope[%d] to be '%s', got '%s'", i, expectedScope, scopes[i])
			}
		}
	})
	t.Run("Parses expired token", func(t *testing.T) {
		expiredClaims, err := ParseJWTClaims(tokenBasicExpired)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if *expiredClaims.Expiry != jwt.NumericDate(1) {
			t.Errorf("expected expiration 1, got %d", basicClaims.Expiry)
		}
	})
}

func TestParseJWTClaimsPayloadInvalid(t *testing.T) {
	t.Run("invalid token segments", func(t *testing.T) {
		invalidToken := "header.payload.signature.extra"

		_, err := ParseJWTClaims(invalidToken)
		if err == nil {
			t.Fatal("expected error for invalid token segments, got nil")
		}

		if !strings.Contains(err.Error(), "compact JWS format must have three parts") {
			t.Errorf("expected invalid token segments error message, got %v", err)
		}
	})
	t.Run("invalid base64 payload", func(t *testing.T) {
		invalidPayload := "invalid_base64" + tokenBasicNotExpired

		_, err := ParseJWTClaims(invalidPayload)
		if err == nil {
			t.Fatal("expected error for invalid base64, got nil")
		}

		if !strings.Contains(err.Error(), "illegal base64 data") {
			t.Errorf("expected decode error message, got %v", err)
		}
	})
}

func TestJWTTokenValidate(t *testing.T) {
	t.Run("expired token returns error", func(t *testing.T) {
		claims, err := ParseJWTClaims(tokenBasicExpired)
		if err != nil {
			t.Fatalf("expected no error for expired token parsing, got %v", err)
		}

		err = claims.Validate("kubernetes-mcp-server")
		if err == nil {
			t.Fatalf("expected error for expired token, got nil")
		}

		if !strings.Contains(err.Error(), "token is expired (exp)") {
			t.Errorf("expected expiration error message, got %v", err)
		}
	})

	t.Run("multiple audiences with correct one", func(t *testing.T) {
		claims, err := ParseJWTClaims(tokenMultipleAudienceNotExpired)
		if err != nil {
			t.Fatalf("expected no error for multiple audience token parsing, got %v", err)
		}
		if claims == nil {
			t.Fatalf("expected claims to be returned, got nil")
		}

		err = claims.Validate("kubernetes-mcp-server")
		if err != nil {
			t.Fatalf("expected no error for valid audience, got %v", err)
		}
	})

	t.Run("multiple audiences with mismatch returns error", func(t *testing.T) {
		claims, err := ParseJWTClaims(tokenMultipleAudienceNotExpired)
		if err != nil {
			t.Fatalf("expected no error for multiple audience token parsing, got %v", err)
		}
		if claims == nil {
			t.Fatalf("expected claims to be returned, got nil")
		}

		err = claims.Validate("missing-audience")
		if err == nil {
			t.Fatalf("expected error for token with wrong audience, got nil")
		}

		if !strings.Contains(err.Error(), "invalid audience claim (aud)") {
			t.Errorf("expected audience mismatch error, got %v", err)
		}
	})
}

func TestJWTClaimsGetScopes(t *testing.T) {
	t.Run("no scopes", func(t *testing.T) {
		claims, err := ParseJWTClaims(tokenBasicExpired)
		if err != nil {
			t.Fatalf("expected no error for parsing token, got %v", err)
		}

		if scopes := claims.GetScopes(); len(scopes) != 0 {
			t.Errorf("expected no scopes, got %d", len(scopes))
		}
	})
	t.Run("single scope", func(t *testing.T) {
		claims := &JWTClaims{
			Scope: "read",
		}
		scopes := claims.GetScopes()
		expected := []string{"read"}

		if len(scopes) != 1 {
			t.Errorf("expected 1 scope, got %d", len(scopes))
		}
		if scopes[0] != expected[0] {
			t.Errorf("expected scope 'read', got '%s'", scopes[0])
		}
	})

	t.Run("multiple scopes", func(t *testing.T) {
		claims := &JWTClaims{
			Scope: "read write admin",
		}
		scopes := claims.GetScopes()
		expected := []string{"read", "write", "admin"}

		if len(scopes) != 3 {
			t.Errorf("expected 3 scopes, got %d", len(scopes))
		}

		for i, expectedScope := range expected {
			if i >= len(scopes) || scopes[i] != expectedScope {
				t.Errorf("expected scope[%d] to be '%s', got '%s'", i, expectedScope, scopes[i])
			}
		}
	})

	t.Run("scopes with extra whitespace", func(t *testing.T) {
		claims := &JWTClaims{
			Scope: "  read   write   admin  ",
		}
		scopes := claims.GetScopes()
		expected := []string{"read", "write", "admin"}

		if len(scopes) != 3 {
			t.Errorf("expected 3 scopes, got %d", len(scopes))
		}

		for i, expectedScope := range expected {
			if i >= len(scopes) || scopes[i] != expectedScope {
				t.Errorf("expected scope[%d] to be '%s', got '%s'", i, expectedScope, scopes[i])
			}
		}
	})
}
