package kubernetes

import (
	"encoding/base64"
	"fmt"
)

// AuthType represents the type of Kubernetes authentication.
type AuthType string

const (
	// AuthTypeToken represents token-based authentication.
	AuthTypeToken AuthType = "token"
	// AuthTypeClientCertificate represents client certificate authentication.
	AuthTypeClientCertificate AuthType = "client_certificate"
	// AuthTypeUnknown represents unknown or unsupported authentication type.
	AuthTypeUnknown       AuthType = "unknown"
	AuthHeadersContextKey string   = "k8s_auth_headers"
)

// K8sAuthHeaders represents Kubernetes API authentication headers.
type K8sAuthHeaders struct {
	// ClusterURL is the Kubernetes cluster URL.
	ClusterURL string
	// ClusterCertificateAuthorityData is the base64-encoded CA certificate.
	ClusterCertificateAuthorityData string
	// AuthorizationToken is the optional bearer token for authentication.
	AuthorizationToken string
	// ClientCertificateData is the optional base64-encoded client certificate.
	ClientCertificateData string
	// ClientKeyData is the optional base64-encoded client key.
	ClientKeyData string
}

func NewK8sAuthHeadersFromHeaders(data map[string]any) (*K8sAuthHeaders, error) {
	authHeaders := &K8sAuthHeaders{}
	var ok bool
	authHeaders.ClusterURL, ok = data[string(CustomClusterURLHeader)].(string)
	if !ok || authHeaders.ClusterURL == "" {
		return nil, fmt.Errorf("%s header is required", CustomClusterURLHeader)
	}

	authHeaders.ClusterCertificateAuthorityData, ok = data[string(CustomCertificateAuthorityDataHeader)].(string)
	if !ok || authHeaders.ClusterCertificateAuthorityData == "" {
		return nil, fmt.Errorf("%s header is required", CustomCertificateAuthorityDataHeader)
	}

	// Token or client certificate and key data (optional).
	authHeaders.AuthorizationToken, _ = data[string(CustomAuthorizationHeader)].(string)
	authHeaders.ClientCertificateData, _ = data[string(CustomClientCertificateDataHeader)].(string)
	authHeaders.ClientKeyData, _ = data[string(CustomClientKeyDataHeader)].(string)

	// Check if either token auth or client certificate auth is provided
	hasTokenAuth := authHeaders.AuthorizationToken != ""
	hasClientCertAuth := authHeaders.ClientCertificateData != "" && authHeaders.ClientKeyData != ""

	if !hasTokenAuth && !hasClientCertAuth {
		return nil, fmt.Errorf("either %s header or (%s and %s) headers are required", CustomAuthorizationHeader, CustomClientCertificateDataHeader, CustomClientKeyDataHeader)
	}

	return authHeaders, nil
}

// GetAuthType returns the authentication type based on the provided headers.
func (h *K8sAuthHeaders) GetAuthType() AuthType {
	if h.AuthorizationToken != "" {
		return AuthTypeToken
	}
	if h.ClientCertificateData != "" && h.ClientKeyData != "" {
		return AuthTypeClientCertificate
	}
	return AuthTypeUnknown
}

// GetDecodedCertificateAuthorityData decodes and returns the CA certificate data.
func (h *K8sAuthHeaders) GetDecodedCertificateAuthorityData() ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(h.ClusterCertificateAuthorityData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode certificate authority data: %w", err)
	}
	return data, nil
}

// // GetDecodedClientCertificateData decodes and returns the client certificate data.
// func (h *K8sAuthHeaders) GetDecodedClientCertificateData() ([]byte, error) {
// 	if h.ClientCertificateData == nil || *h.ClientCertificateData == "" {
// 		return nil, errors.New("client certificate data is not available")
// 	}
// 	data, err := base64.StdEncoding.DecodeString(*h.ClientCertificateData)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode client certificate data: %w", err)
// 	}
// 	return data, nil
// }

// // GetDecodedClientKeyData decodes and returns the client key data.
// func (h *K8sAuthHeaders) GetDecodedClientKeyData() ([]byte, error) {
// 	if h.ClientKeyData == nil || *h.ClientKeyData == "" {
// 		return nil, errors.New("client key data is not available")
// 	}
// 	data, err := base64.StdEncoding.DecodeString(*h.ClientKeyData)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode client key data: %w", err)
// 	}
// 	return data, nil
// }
