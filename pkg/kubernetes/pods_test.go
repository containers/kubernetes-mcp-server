package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
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
