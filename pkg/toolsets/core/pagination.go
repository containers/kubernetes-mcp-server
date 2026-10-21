package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// ListPageSentence is appended to a list tool or prompt description.
const ListPageSentence = " Use limit and continue to retrieve a large list one page at a time."

const (
	// LimitDescription documents the shared limit argument.
	LimitDescription = "Maximum number of items to return. Omit or set to 0 to list everything, which fails if the response exceeds max_backend_response_bytes. Retry with a smaller limit when that happens. The API may return fewer items than requested."
	// ContinueDescription documents the shared continue argument.
	// It matches the Kubernetes list continue token.
	ContinueDescription = "Opaque token from the continue field of the previous page. Send it unchanged with the same limit, namespace, and selectors to retrieve the next page. Omit on the first call."
)

var (
	limitSchema = &jsonschema.Schema{
		Type:        "integer",
		Description: LimitDescription,
		Minimum:     ptr.To(float64(0)),
	}
	continueSchema = &jsonschema.Schema{
		Type:        "string",
		Description: ContinueDescription,
	}
)

// PageProperties adds limit and continue to a tool input schema.
func PageProperties(properties map[string]*jsonschema.Schema) map[string]*jsonschema.Schema {
	if properties == nil {
		properties = map[string]*jsonschema.Schema{}
	}
	properties["limit"] = limitSchema
	properties["continue"] = continueSchema
	return properties
}

// ApplyListPage copies limit and continue onto opts.
// A negative limit is an argument error. Zero leaves the list uncapped.
func ApplyListPage(p *api.Params, opts *metav1.ListOptions) error {
	limit := p.OptionalInt64("limit", 0)
	cont := p.OptionalString("continue", "")
	if err := p.Err(); err != nil {
		return err
	}
	if limit < 0 {
		return fmt.Errorf("limit must be greater than or equal to 0")
	}
	opts.Limit = limit
	opts.Continue = cont
	return nil
}

// ListErr adds the retry hint when a list body exceeds max_backend_response_bytes.
func ListErr(err error) error {
	if kubernetes.IsResponseTooLarge(err) {
		return fmt.Errorf("%w; retry with a smaller limit", err)
	}
	return err
}

type listContinue interface {
	GetContinue() string
	GetRemainingItemCount() *int64
}

// WithListPage prepends the continue token from listed when the API returned one.
// Structured content keeps its items and gains continue, and remainingItemCount when set.
func WithListPage(text string, structured any, listed any) (string, any) {
	if listed == nil {
		return text, structured
	}
	page, ok := listed.(listContinue)
	if !ok {
		return text, structured
	}
	return attachContinue(text, structured, page.GetContinue(), page.GetRemainingItemCount())
}

func attachContinue(text string, structured any, cont string, remaining *int64) (string, any) {
	if cont == "" {
		return text, structured
	}
	header := "# continue: " + cont + "\n"
	if remaining != nil {
		header += fmt.Sprintf("# remainingItemCount: %d\n", *remaining)
	}
	text = header + text

	switch s := structured.(type) {
	case map[string]any:
		s["continue"] = cont
		if remaining != nil {
			s["remainingItemCount"] = *remaining
		}
		return text, s
	case nil:
		page := map[string]any{"continue": cont}
		if remaining != nil {
			page["remainingItemCount"] = *remaining
		}
		return text, page
	default:
		page := map[string]any{"items": structured, "continue": cont}
		if remaining != nil {
			page["remainingItemCount"] = *remaining
		}
		return text, page
	}
}
