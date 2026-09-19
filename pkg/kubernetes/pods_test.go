package kubernetes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResolveContainerSuite struct {
	suite.Suite
}

func (s *ResolveContainerSuite) TestResolveContainer() {
	s.Run("explicit container is returned as-is", func() {
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "main"},
					{Name: "sidecar"},
				},
			},
		}
		s.Equal("explicit", resolveContainer(pod, "explicit"))
	})
	s.Run("single container pod returns that container", func() {
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "only-container"},
				},
			},
		}
		s.Equal("only-container", resolveContainer(pod, ""))
	})
	s.Run("multi-container pod with annotation returns annotated container", func() {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					DefaultContainerAnnotation: "sidecar",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "main"},
					{Name: "sidecar"},
				},
			},
		}
		s.Equal("sidecar", resolveContainer(pod, ""))
	})
	s.Run("multi-container pod without annotation falls back to first container", func() {
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "first"},
					{Name: "second"},
				},
			},
		}
		s.Equal("first", resolveContainer(pod, ""))
	})
	s.Run("annotation with empty value falls back to first container", func() {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					DefaultContainerAnnotation: "",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "first"},
					{Name: "second"},
				},
			},
		}
		s.Equal("first", resolveContainer(pod, ""))
	})
	s.Run("pod with no containers returns empty string", func() {
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{},
			},
		}
		s.Equal("", resolveContainer(pod, ""))
	})
	s.Run("explicit container takes precedence over annotation", func() {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					DefaultContainerAnnotation: "sidecar",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "main"},
					{Name: "sidecar"},
				},
			},
		}
		s.Equal("main", resolveContainer(pod, "main"))
	})
}

func TestResolveContainer(t *testing.T) {
	suite.Run(t, new(ResolveContainerSuite))
}

type ExtractKubernetesErrorSuite struct {
	suite.Suite
}

func (s *ExtractKubernetesErrorSuite) TestExtractKubernetesError() {
	s.Run("non-StatusError is returned as-is", func() {
		original := fmt.Errorf("some network error")
		s.Equal(original, extractKubernetesError(original))
	})
	s.Run("wrapped non-StatusError is returned as-is", func() {
		inner := fmt.Errorf("connection refused")
		wrapped := fmt.Errorf("request failed: %w", inner)
		s.Equal(wrapped, extractKubernetesError(wrapped))
	})
	s.Run("StatusError with non-empty message is returned as-is", func() {
		err := &apierrors.StatusError{ErrStatus: metav1.Status{
			Message: "pods \"my-pod\" not found",
			Reason:  metav1.StatusReasonNotFound,
			Code:    404,
		}}
		s.Equal(err, extractKubernetesError(err))
	})
	s.Run("StatusError with empty message extracts cause", func() {
		err := &apierrors.StatusError{ErrStatus: metav1.Status{
			Reason: metav1.StatusReasonBadRequest,
			Code:   400,
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{
					{Message: `container "buggy-app" in pod "buggy-app-abc123" not found`},
				},
			},
		}}
		result := extractKubernetesError(err)
		s.Contains(result.Error(), "BadRequest")
		s.Contains(result.Error(), `container "buggy-app" in pod "buggy-app-abc123" not found`)
	})
	s.Run("StatusError with empty message and multiple causes uses first non-empty", func() {
		err := &apierrors.StatusError{ErrStatus: metav1.Status{
			Reason: metav1.StatusReasonBadRequest,
			Code:   400,
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{
					{Message: ""},
					{Message: "the real error message"},
				},
			},
		}}
		result := extractKubernetesError(err)
		s.Contains(result.Error(), "the real error message")
	})
	s.Run("StatusError with empty message and no details falls back to reason and code", func() {
		err := &apierrors.StatusError{ErrStatus: metav1.Status{
			Reason: metav1.StatusReasonBadRequest,
			Code:   400,
		}}
		s.Equal("BadRequest (HTTP 400)", extractKubernetesError(err).Error())
	})
	s.Run("StatusError with empty message and empty causes falls back to reason and code", func() {
		err := &apierrors.StatusError{ErrStatus: metav1.Status{
			Reason: metav1.StatusReasonBadRequest,
			Code:   400,
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{},
			},
		}}
		s.Equal("BadRequest (HTTP 400)", extractKubernetesError(err).Error())
	})
	s.Run("wrapped StatusError with empty message extracts cause", func() {
		inner := &apierrors.StatusError{ErrStatus: metav1.Status{
			Reason: metav1.StatusReasonBadRequest,
			Code:   400,
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{
					{Message: "container not found"},
				},
			},
		}}
		wrapped := fmt.Errorf("log retrieval failed: %w", inner)
		result := extractKubernetesError(wrapped)
		s.NotEqual(wrapped, result)
		s.Contains(result.Error(), "container not found")
	})
}

func TestExtractKubernetesError(t *testing.T) {
	suite.Run(t, new(ExtractKubernetesErrorSuite))
}
