package tekton

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type coreOnlyClient struct {
	corev1client.CoreV1Interface
}

func (c coreOnlyClient) CoreV1() corev1client.CoreV1Interface {
	return c.CoreV1Interface
}

func TestReadContainerLogBoundsOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/namespaces/test/pods/pod/log", r.URL.Path)
		require.Equal(t, "step-main", r.URL.Query().Get("container"))
		require.Equal(t, "100", r.URL.Query().Get("tailLines"))
		_, _ = fmt.Fprint(w, "0123456789")
	}))
	defer server.Close()

	client, err := corev1client.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	logText, truncated, err := readContainerLog(context.Background(), coreOnlyClient{client}, "test", "pod", "step-main", 5, 100)
	require.NoError(t, err)
	require.Equal(t, "01234", logText)
	require.True(t, truncated)
}

func TestReadContainerLogReturnsRBACError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"pods/log is forbidden","reason":"Forbidden","code":403}`)
	}))
	defer server.Close()

	client, err := corev1client.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	_, _, err = readContainerLog(context.Background(), coreOnlyClient{client}, "test", "pod", "step-main", 5)
	require.ErrorContains(t, err, "pods/log is forbidden")
}

func TestSanitizeDiagnosisText(t *testing.T) {
	text, truncated := sanitizeDiagnosisText("Authorization: Bearer secret-token\ntoken=another-secret")
	require.NotContains(t, text, "secret-token")
	require.NotContains(t, text, "another-secret")
	require.False(t, truncated)

	diagnosis := &pipelineRunDiagnosis{}
	text = diagnosis.sanitizeText(string(make([]rune, maxDiagnosisMessageRunes+1)))
	require.True(t, utf8.ValidString(text))
	require.Contains(t, text, "[truncated]")
	require.True(t, diagnosis.Truncated)
}

func TestTruncateUTF8Bytes(t *testing.T) {
	text, truncated := truncateUTF8Bytes(strings.Repeat("界", 3), 5)
	require.True(t, truncated)
	require.True(t, utf8.ValidString(text))
	require.LessOrEqual(t, len(text), 5)
}
