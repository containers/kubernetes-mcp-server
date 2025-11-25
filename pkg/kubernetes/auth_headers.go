package kubernetes

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// AuthType represents the type of Kubernetes authentication.
type AuthType string
type ContextKey string

const (
	// AuthTypeToken represents token-based authentication.
	AuthTypeToken AuthType = "token"
	// AuthTypeClientCertificate represents client certificate authentication.
	AuthTypeClientCertificate AuthType = "client_certificate"
	// AuthHeadersContextKey is the context key for the Kubernetes authentication headers.
	AuthHeadersContextKey ContextKey = "k8s_auth_headers"
)

// K8sAuthHeaders represents Kubernetes API authentication headers.
type K8sAuthHeaders struct {
	// Server is the Kubernetes cluster URL.
	Server string
	// ClusterCertificateAuthorityData is the Certificate Authority data.
	CertificateAuthorityData []byte
	// AuthorizationToken is the optional bearer token for authentication.
	AuthorizationToken string
	// ClientCertificateData is the optional client certificate data.
	ClientCertificateData []byte
	// ClientKeyData is the optional client key data.
	ClientKeyData []byte
	// InsecureSkipTLSVerify is the optional flag to skip TLS verification.
	InsecureSkipTLSVerify bool
}

// GetDecodedData decodes and returns the data.
func GetDecodedData(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func NewK8sAuthHeadersFromHeaders(data map[string]any) (*K8sAuthHeaders, error) {
	var ok bool
	var err error

	// Initialize auth headers.
	authHeaders := &K8sAuthHeaders{
		InsecureSkipTLSVerify: false,
	}

	// Get cluster URL from headers.
	authHeaders.Server, ok = data[string(CustomServerHeader)].(string)
	if !ok || authHeaders.Server == "" {
		return nil, fmt.Errorf("%s header is required", CustomServerHeader)
	}

	// Get certificate authority data from headers.
	certificateAuthorityDataBase64, ok := data[string(CustomCertificateAuthorityDataHeader)].(string)
	if !ok || certificateAuthorityDataBase64 == "" {
		return nil, fmt.Errorf("%s header is required", CustomCertificateAuthorityDataHeader)
	}
	// Decode certificate authority data.
	authHeaders.CertificateAuthorityData, err = GetDecodedData(certificateAuthorityDataBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate authority data: %w", err)
	}

	// Get insecure skip TLS verify flag from headers.
	if data[string(CustomInsecureSkipTLSVerifyHeader)] != nil && strings.ToLower(data[string(CustomInsecureSkipTLSVerifyHeader)].(string)) == "true" {
		authHeaders.InsecureSkipTLSVerify = true
	}

	// Get authorization token from headers.
	authHeaders.AuthorizationToken, _ = data[string(CustomAuthorizationHeader)].(string)

	// Get client certificate data from headers.
	clientCertificateDataBase64, _ := data[string(CustomClientCertificateDataHeader)].(string)
	if clientCertificateDataBase64 != "" {
		authHeaders.ClientCertificateData, err = GetDecodedData(clientCertificateDataBase64)
		if err != nil {
			return nil, fmt.Errorf("invalid client certificate data: %w", err)
		}
	}
	// Get client key data from headers.
	clientKeyDataBase64, _ := data[string(CustomClientKeyDataHeader)].(string)
	if clientKeyDataBase64 != "" {
		authHeaders.ClientKeyData, err = GetDecodedData(clientKeyDataBase64)
		if err != nil {
			return nil, fmt.Errorf("invalid client key data: %w", err)
		}
	}

	// Check if a valid authentication type is provided.
	_, err = authHeaders.GetAuthType()
	if err != nil {
		return nil, fmt.Errorf("either %s header for token authentication or (%s and %s) headers for client certificate authentication required", CustomAuthorizationHeader, CustomClientCertificateDataHeader, CustomClientKeyDataHeader)
	}

	return authHeaders, nil
}

// GetAuthType returns the authentication type based on the provided headers.
func (h *K8sAuthHeaders) GetAuthType() (AuthType, error) {
	if h.AuthorizationToken != "" {
		return AuthTypeToken, nil
	}
	if h.ClientCertificateData != nil && h.ClientKeyData != nil {
		return AuthTypeClientCertificate, nil
	}
	return "", fmt.Errorf("invalid authentication type")
}
