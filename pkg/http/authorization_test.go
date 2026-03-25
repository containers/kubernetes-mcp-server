package http

import (
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	// https://jwt.io/#token=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJtY3Atc2VydmVyIl0sImV4cCI6MjUzNDAyMjk3MTk5LCJpYXQiOjAsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOTkyMjJkNTYtMzQwZS00ZWI2LTg1ODgtMjYxNDExZjM1ZDI2Iiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImRlZmF1bHQiLCJ1aWQiOiJlYWNiNmFkMi04MGI3LTQxNzktODQzZC05MmViMWU2YmJiYTYifX0sIm5iZiI6MCwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGVmYXVsdCJ9.ld9aJaQX5k44KOV1bv8MCY2RceAZ9jAjN2vKswKmINNiOpRMl0f8Y0trrq7gdRlKwGLsCUjz8hbHsGcM43QtNrcwfvH5imRnlAKANPUgswwEadCTjASihlo6ADsn9fjAWB4viplFwq8VdzcwpcyActYJi2TBFoRq204STZJIcAW_B40HOuCB2XxQ81V4_XWLzL03Bt-YmYUhliiiE5YSKS1WEEWIbdel--b7Gvp-VS1I2eeiOqV3SelMBHbF9EwKGAkyObg0JhGqr5XHLd6WOmhvLus4eCkyakQMgr2tZIdvbt2yEUDiId6r27tlgAPLmqlyYMEhyiM212_Sth3T3Q // notsecret
	tokenBasicNotExpired = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJtY3Atc2VydmVyIl0sImV4cCI6MjUzNDAyMjk3MTk5LCJpYXQiOjAsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOTkyMjJkNTYtMzQwZS00ZWI2LTg1ODgtMjYxNDExZjM1ZDI2Iiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImRlZmF1bHQiLCJ1aWQiOiJlYWNiNmFkMi04MGI3LTQxNzktODQzZC05MmViMWU2YmJiYTYifX0sIm5iZiI6MCwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGVmYXVsdCJ9.ld9aJaQX5k44KOV1bv8MCY2RceAZ9jAjN2vKswKmINNiOpRMl0f8Y0trrq7gdRlKwGLsCUjz8hbHsGcM43QtNrcwfvH5imRnlAKANPUgswwEadCTjASihlo6ADsn9fjAWB4viplFwq8VdzcwpcyActYJi2TBFoRq204STZJIcAW_B40HOuCB2XxQ81V4_XWLzL03Bt-YmYUhliiiE5YSKS1WEEWIbdel--b7Gvp-VS1I2eeiOqV3SelMBHbF9EwKGAkyObg0JhGqr5XHLd6WOmhvLus4eCkyakQMgr2tZIdvbt2yEUDiId6r27tlgAPLmqlyYMEhyiM212_Sth3T3Q" // notsecret
	// https://jwt.io/#token=eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJtY3Atc2VydmVyIl0sImV4cCI6MSwiaWF0IjowLCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImp0aSI6Ijk5MjIyZDU2LTM0MGUtNGViNi04NTg4LTI2MTQxMWYzNWQyNiIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoiZGVmYXVsdCIsInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiZWFjYjZhZDItODBiNy00MTc5LTg0M2QtOTJlYjFlNmJiYmE2In19LCJuYmYiOjAsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmRlZmF1bHQifQ.iVrxt6glbY3Qe_mEtK-lYpx4Z3VC1a7zgGRSmfu29pMmnKhlTk56y0Wx45DQ4PSYCTwC6CJnGGZNbJyr4JS8PQ // notsecret
	tokenBasicExpired = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJtY3Atc2VydmVyIl0sImV4cCI6MSwiaWF0IjowLCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImp0aSI6Ijk5MjIyZDU2LTM0MGUtNGViNi04NTg4LTI2MTQxMWYzNWQyNiIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoiZGVmYXVsdCIsInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJkZWZhdWx0IiwidWlkIjoiZWFjYjZhZDItODBiNy00MTc5LTg0M2QtOTJlYjFlNmJiYmE2In19LCJuYmYiOjAsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmRlZmF1bHQifQ.iVrxt6glbY3Qe_mEtK-lYpx4Z3VC1a7zgGRSmfu29pMmnKhlTk56y0Wx45DQ4PSYCTwC6CJnGGZNbJyr4JS8PQ" // notsecret
	// https://jwt.io/#token=eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJtY3Atc2VydmVyIl0sImV4cCI6MjUzNDAyMjk3MTk5LCJpYXQiOjAsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOTkyMjJkNTYtMzQwZS00ZWI2LTg1ODgtMjYxNDExZjM1ZDI2Iiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImRlZmF1bHQiLCJ1aWQiOiJlYWNiNmFkMi04MGI3LTQxNzktODQzZC05MmViMWU2YmJiYTYifX0sIm5iZiI6MCwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGVmYXVsdCIsInNjb3BlIjoicmVhZCB3cml0ZSJ9.m5mFXp0TDSvgLevQ76nX65N14w1RxTClMaannLLOuBIUEsmXhMYZjGtf5mWMcxVOkSh65rLFiKugaMXgv877Mg // notsecret
	tokenMultipleAudienceNotExpired = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiIsImtpZCI6Ijk4ZDU3YmUwNWI3ZjUzNWIwMzYyYjg2MDJhNTJlNGYxIn0.eyJhdWQiOlsiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJtY3Atc2VydmVyIl0sImV4cCI6MjUzNDAyMjk3MTk5LCJpYXQiOjAsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOTkyMjJkNTYtMzQwZS00ZWI2LTg1ODgtMjYxNDExZjM1ZDI2Iiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImRlZmF1bHQiLCJ1aWQiOiJlYWNiNmFkMi04MGI3LTQxNzktODQzZC05MmViMWU2YmJiYTYifX0sIm5iZiI6MCwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGVmYXVsdCIsInNjb3BlIjoicmVhZCB3cml0ZSJ9.m5mFXp0TDSvgLevQ76nX65N14w1RxTClMaannLLOuBIUEsmXhMYZjGtf5mWMcxVOkSh65rLFiKugaMXgv877Mg" // notsecret
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
		expectedAudiences := []string{"https://kubernetes.default.svc.cluster.local", "mcp-server"}
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
		invalidPayload := strings.ReplaceAll(tokenBasicNotExpired, ".", ".invalid")

		_, err := ParseJWTClaims(invalidPayload)
		if err == nil {
			t.Fatal("expected error for invalid base64, got nil")
		}

		if !strings.Contains(err.Error(), "illegal base64 data") {
			t.Errorf("expected decode error message, got %v", err)
		}
	})
}

func TestJWTTokenValidateOffline(t *testing.T) {
	t.Run("expired token returns error", func(t *testing.T) {
		claims, err := ParseJWTClaims(tokenBasicExpired)
		if err != nil {
			t.Fatalf("expected no error for expired token parsing, got %v", err)
		}

		err = claims.ValidateOffline("mcp-server")
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

		err = claims.ValidateOffline("mcp-server")
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

		err = claims.ValidateOffline("missing-audience")
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

	t.Run("Entra ID scp claim", func(t *testing.T) {
		claims := &JWTClaims{
			Scp: "read write",
		}
		scopes := claims.GetScopes()
		expected := []string{"read", "write"}

		if len(scopes) != 2 {
			t.Errorf("expected 2 scopes, got %d", len(scopes))
		}
		for i, expectedScope := range expected {
			if i >= len(scopes) || scopes[i] != expectedScope {
				t.Errorf("expected scope[%d] to be '%s', got '%s'", i, expectedScope, scopes[i])
			}
		}
	})

	t.Run("standard scope takes precedence over scp", func(t *testing.T) {
		claims := &JWTClaims{
			Scope: "admin",
			Scp:   "read write",
		}
		scopes := claims.GetScopes()

		if len(scopes) != 1 {
			t.Errorf("expected 1 scope, got %d", len(scopes))
		}
		if scopes[0] != "admin" {
			t.Errorf("expected scope 'admin' (from standard claim), got '%s'", scopes[0])
		}
	})
}
