package kubernetes

import (
	"context"
	"errors"
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

type ExecOutputSuite struct {
	suite.Suite
}

func (s *ExecOutputSuite) TestKeepsBytesAndAppendsNotice() {
	ctx, cancel := context.WithCancel(context.Background())
	out := newExecOutput(5, cancel)
	_, err := out.stdoutWriter().Write([]byte("abc"))
	s.Require().NoError(err)
	_, err = out.stderrWriter().Write([]byte("defg"))
	s.Require().NoError(err)
	s.ErrorIs(ctx.Err(), context.Canceled)

	stdout, stderr, err := out.finish(context.Canceled)
	s.Require().NoError(err)
	s.Equal("abc"+backendExecTruncationNotice, stdout)
	s.Equal("de", stderr)
}

func (s *ExecOutputSuite) TestExactFitDoesNotTruncate() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newExecOutput(4, cancel)
	_, err := out.stdoutWriter().Write([]byte("ab"))
	s.Require().NoError(err)
	_, err = out.stderrWriter().Write([]byte("cd"))
	s.Require().NoError(err)
	stdout, stderr, err := out.finish(nil)
	s.Require().NoError(err)
	s.Equal("ab", stdout)
	s.Equal("cd", stderr)
}

func (s *ExecOutputSuite) TestOtherStreamErrorDropsBuffers() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newExecOutput(4, cancel)
	_, err := out.stdoutWriter().Write([]byte("abcdef"))
	s.Require().NoError(err)
	streamErr := errors.New("connection reset")
	stdout, stderr, err := out.finish(streamErr)
	s.ErrorIs(err, streamErr)
	s.Empty(stdout)
	s.Empty(stderr)
}

func (s *ExecOutputSuite) TestDisabledLimitKeepsEverything() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newExecOutput(0, cancel)
	_, err := out.stdoutWriter().Write([]byte("abcdef"))
	s.Require().NoError(err)
	stdout, stderr, err := out.finish(nil)
	s.Require().NoError(err)
	s.Equal("abcdef", stdout)
	s.Empty(stderr)
}

func (s *ExecOutputSuite) TestMissingConfigErrors() {
	_, _, err := NewCore(nil).PodsExec(context.Background(), nil, "", "", "", nil)
	s.ErrorIs(err, ErrBackendLimitUnavailable)
}

func TestExecOutput(t *testing.T) {
	suite.Run(t, new(ExecOutputSuite))
}
