package kubernetes

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDecodedData(t *testing.T) {
	t.Run("decodes valid base64 string", func(t *testing.T) {
		input := "SGVsbG8gV29ybGQ=" // "Hello World" in base64
		expected := []byte("Hello World")

		result, err := GetDecodedData(input)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("decodes empty string", func(t *testing.T) {
		input := ""
		expected := []byte{}

		result, err := GetDecodedData(input)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("returns error for invalid base64", func(t *testing.T) {
		input := "not-valid-base64!!!"

		_, err := GetDecodedData(input)
		require.Error(t, err)
	})

	t.Run("decodes base64 with padding", func(t *testing.T) {
		input := "dGVzdA==" // "test" in base64
		expected := []byte("test")

		result, err := GetDecodedData(input)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})
}

func TestNewK8sAuthHeadersFromHeaders(t *testing.T) {
	serverURL := "https://kubernetes.example.com:6443"
	caCert := []byte("test-ca-cert")
	caCertBase64 := base64.StdEncoding.EncodeToString(caCert)
	token := "Bearer test-token"
	clientCert := []byte("test-client-cert")
	clientCertBase64 := base64.StdEncoding.EncodeToString(clientCert)
	clientKey := []byte("test-client-key")
	clientKeyBase64 := base64.StdEncoding.EncodeToString(clientKey)

	t.Run("creates auth headers with token authentication", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		require.NotNil(t, authHeaders)

		assert.Equal(t, serverURL, authHeaders.Server)
		assert.Equal(t, caCert, authHeaders.CertificateAuthorityData)
		assert.Equal(t, token, authHeaders.AuthorizationToken)
		assert.Nil(t, authHeaders.ClientCertificateData)
		assert.Nil(t, authHeaders.ClientKeyData)
		assert.False(t, authHeaders.InsecureSkipTLSVerify)
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("creates auth headers with client certificate authentication", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomClientCertificateDataHeader):    clientCertBase64,
			string(CustomClientKeyDataHeader):            clientKeyBase64,
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		require.NotNil(t, authHeaders)

		assert.Equal(t, serverURL, authHeaders.Server)
		assert.Equal(t, caCert, authHeaders.CertificateAuthorityData)
		assert.Equal(t, "", authHeaders.AuthorizationToken)
		assert.Equal(t, clientCert, authHeaders.ClientCertificateData)
		assert.Equal(t, clientKey, authHeaders.ClientKeyData)
		assert.False(t, authHeaders.InsecureSkipTLSVerify)
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("creates auth headers with both token and client certificate", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
			string(CustomClientCertificateDataHeader):    clientCertBase64,
			string(CustomClientKeyDataHeader):            clientKeyBase64,
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		require.NotNil(t, authHeaders)

		// Should have both auth methods
		assert.Equal(t, token, authHeaders.AuthorizationToken)
		assert.Equal(t, clientCert, authHeaders.ClientCertificateData)
		assert.Equal(t, clientKey, authHeaders.ClientKeyData)
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("sets InsecureSkipTLSVerify to true when header is 'true'", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
			string(CustomInsecureSkipTLSVerifyHeader):    "true",
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.True(t, authHeaders.InsecureSkipTLSVerify)
	})

	t.Run("sets InsecureSkipTLSVerify to true when header is 'TRUE' (case insensitive)", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
			string(CustomInsecureSkipTLSVerifyHeader):    "TRUE",
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.True(t, authHeaders.InsecureSkipTLSVerify)
	})

	t.Run("sets InsecureSkipTLSVerify to false when header is 'false'", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
			string(CustomInsecureSkipTLSVerifyHeader):    "false",
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.False(t, authHeaders.InsecureSkipTLSVerify)
	})

	t.Run("sets InsecureSkipTLSVerify to false when header is missing", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.False(t, authHeaders.InsecureSkipTLSVerify)
	})

	t.Run("returns error when server header is missing", func(t *testing.T) {
		headers := map[string]any{
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-server")
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("returns error when server header is empty string", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   "",
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-server")
	})

	t.Run("returns error when server header is not a string", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   123,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-server")
	})

	t.Run("returns error when CA data header is missing", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):        serverURL,
			string(CustomAuthorizationHeader): token,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-certificate-authority-data")
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("returns error when CA data header is empty string", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): "",
			string(CustomAuthorizationHeader):            token,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-certificate-authority-data")
	})

	t.Run("returns error when CA data is invalid base64", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): "invalid-base64!!!",
			string(CustomAuthorizationHeader):            token,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid certificate authority data")
	})

	t.Run("returns error when no authentication method is provided", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication")
		assert.Contains(t, err.Error(), "kubernetes-authorization")
		assert.Contains(t, err.Error(), "kubernetes-client-certificate-data")
		assert.Contains(t, err.Error(), "kubernetes-client-key-data")
	})

	t.Run("returns error when only client certificate is provided without key", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomClientCertificateDataHeader):    clientCertBase64,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication")
	})

	t.Run("returns error when only client key is provided without certificate", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomClientKeyDataHeader):            clientKeyBase64,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication")
	})

	t.Run("returns error when client certificate is invalid base64", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomClientCertificateDataHeader):    "invalid-base64!!!",
			string(CustomClientKeyDataHeader):            clientKeyBase64,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid client certificate data")
	})

	t.Run("returns error when client key is invalid base64", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomClientCertificateDataHeader):    clientCertBase64,
			string(CustomClientKeyDataHeader):            "invalid-base64!!!",
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid client key data")
	})

	t.Run("handles empty token string gracefully", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            "",
			string(CustomClientCertificateDataHeader):    clientCertBase64,
			string(CustomClientKeyDataHeader):            clientKeyBase64,
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		// Empty token is OK if we have client cert
		assert.Equal(t, "", authHeaders.AuthorizationToken)
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("handles empty client cert/key strings gracefully when token is provided", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   serverURL,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            token,
			string(CustomClientCertificateDataHeader):    "",
			string(CustomClientKeyDataHeader):            "",
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.Nil(t, authHeaders.ClientCertificateData)
		assert.Nil(t, authHeaders.ClientKeyData)
		assert.True(t, authHeaders.IsValid())
	})
}

func TestK8sAuthHeaders_IsValid(t *testing.T) {
	t.Run("returns true when token is provided", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			AuthorizationToken: "Bearer test-token",
		}
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("returns true when client certificate and key are provided", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			ClientCertificateData: []byte("cert-data"),
			ClientKeyData:         []byte("key-data"),
		}
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("returns true when both token and client cert are provided", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			AuthorizationToken:    "Bearer test-token",
			ClientCertificateData: []byte("cert-data"),
			ClientKeyData:         []byte("key-data"),
		}
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("returns false when no authentication is provided", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{}
		assert.False(t, authHeaders.IsValid())
	})

	t.Run("returns false when only client certificate is provided", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			ClientCertificateData: []byte("cert-data"),
		}
		assert.False(t, authHeaders.IsValid())
	})

	t.Run("returns false when only client key is provided", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			ClientKeyData: []byte("key-data"),
		}
		assert.False(t, authHeaders.IsValid())
	})

	t.Run("returns false when token is empty string", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			AuthorizationToken: "",
		}
		assert.False(t, authHeaders.IsValid())
	})

	t.Run("returns false when client cert and key are empty slices", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			ClientCertificateData: []byte{},
			ClientKeyData:         []byte{},
		}
		// Empty slices have length 0, so they're considered invalid
		assert.False(t, authHeaders.IsValid())
	})

	t.Run("returns false when client cert is nil and key has data", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			ClientCertificateData: nil,
			ClientKeyData:         []byte("key-data"),
		}
		assert.False(t, authHeaders.IsValid())
	})

	t.Run("returns false when client cert has data and key is nil", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			ClientCertificateData: []byte("cert-data"),
			ClientKeyData:         nil,
		}
		assert.False(t, authHeaders.IsValid())
	})
}
