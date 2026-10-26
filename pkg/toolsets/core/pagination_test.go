package core

import (
	"testing"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type PaginationSuite struct {
	suite.Suite
}

func TestPagination(t *testing.T) {
	suite.Run(t, new(PaginationSuite))
}

type argRequest struct {
	args map[string]any
}

func (a argRequest) GetArguments() map[string]any {
	return a.args
}

func (s *PaginationSuite) params(args map[string]any) *api.Params {
	return api.WrapParams(api.ToolHandlerParams{Request: argRequest{args: args}})
}

func (s *PaginationSuite) TestApplyListPage() {
	s.Run("omitted arguments leave the list uncapped", func() {
		opts := metav1.ListOptions{}
		s.NoError(ApplyListPage(s.params(map[string]any{}), &opts))
		s.Equal(int64(0), opts.Limit)
		s.Empty(opts.Continue)
	})
	s.Run("copies limit and continue", func() {
		opts := metav1.ListOptions{}
		s.NoError(ApplyListPage(s.params(map[string]any{
			"limit":    float64(2),
			"continue": "next",
		}), &opts))
		s.Equal(int64(2), opts.Limit)
		s.Equal("next", opts.Continue)
	})
	s.Run("rejects a negative limit", func() {
		opts := metav1.ListOptions{}
		err := ApplyListPage(s.params(map[string]any{"limit": float64(-1)}), &opts)
		s.EqualError(err, "limit must be greater than or equal to 0")
		s.Equal(int64(0), opts.Limit)
	})
	s.Run("rejects a limit that is not an integer", func() {
		err := ApplyListPage(s.params(map[string]any{"limit": "2"}), &metav1.ListOptions{})
		s.ErrorContains(err, "limit parameter must be an integer")
	})
}

func (s *PaginationSuite) TestListErrHintsAtLimit() {
	err := ListErr(&kubernetes.BackendResponseTooLargeError{Limit: 8})
	s.ErrorContains(err, "retry with a smaller limit")
	s.True(kubernetes.IsResponseTooLarge(err))
	s.EqualError(ListErr(errSentinel), errSentinel.Error())
}

var errSentinel = errString("list failed")

type errString string

func (e errString) Error() string { return string(e) }

func (s *PaginationSuite) TestAttachContinue() {
	items := []map[string]any{{"name": "a"}}
	s.Run("leaves a complete list unchanged", func() {
		text, structured := attachContinue("items", items, "", nil)
		s.Equal("items", text)
		s.Equal(items, structured)
	})
	s.Run("wraps a slice and records the token", func() {
		remaining := ptr.To(int64(4))
		text, structured := attachContinue("items\n", items, "tok", remaining)
		s.Equal("# continue: tok\n# remainingItemCount: 4\nitems\n", text)
		page, ok := structured.(map[string]any)
		s.Require().True(ok)
		s.Equal(items, page["items"])
		s.Equal("tok", page["continue"])
		s.Equal(int64(4), page["remainingItemCount"])
	})
	s.Run("adds the token to an existing object", func() {
		rows := map[string]any{"items": items, "columns": []string{"Name"}}
		text, structured := WithListPage("table", rows, &unstructured.UnstructuredList{
			Object: map[string]any{"metadata": map[string]any{"continue": "tok"}},
		})
		s.Equal("# continue: tok\ntable", text)
		s.Equal("tok", structured.(map[string]any)["continue"])
		s.Equal(items, structured.(map[string]any)["items"])
	})
}

func (s *PaginationSuite) TestPageProperties() {
	props := PageProperties(nil)
	s.Equal("integer", props["limit"].Type)
	s.Equal("string", props["continue"].Type)
	s.Contains(props["limit"].Description, "max_backend_response_bytes")
}
