package confluence

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	goconfluence "github.com/virtomize/confluence-go-api"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// NewToolset returns a new toolset for Confluence.
func NewToolset(cfg *config.ConfluenceConfig) (api.Toolset, error) {
	if cfg == nil || cfg.URL == "" {
		return &disabledToolset{}, nil
	}

	confluenceAPI, err := goconfluence.NewAPI(cfg.URL, cfg.Username, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create confluence api: %w", err)
	}

	return &confluenceToolset{api: confluenceAPI}, nil
}

type confluenceToolset struct {
	api *goconfluence.API
}

func (t *confluenceToolset) GetName() string {
	return "confluence"
}

func (t *confluenceToolset) GetDescription() string {
	return "Tools for interacting with Confluence"
}

func (t *confluenceToolset) GetTools(_ kubernetes.Openshift) []api.ServerTool {
	if t.api == nil {
		return nil
	}
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "confluence.createPage",
				Description: "Create a new page in Confluence.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"space_key": {
							Type:        "string",
							Description: "The key of the space to create the page in.",
						},
						"title": {
							Type:        "string",
							Description: "The title of the new page.",
						},
						"content": {
							Type:        "string",
							Description: "The content of the new page in Confluence Storage Format (XHTML).",
						},
						"parent_id": {
							Type:        "string",
							Description: "Optional ID of a parent page.",
						},
					},
					Required: []string{"space_key", "title", "content"},
				},
				Annotations: api.ToolAnnotations{
					DestructiveHint: ptr.To(true),
				},
			},
			Handler: createPageHandler(t.api),
		},
	}
}

func createPageHandler(confluenceAPI *goconfluence.API) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		spaceKey, _ := params.GetArguments()["space_key"].(string)
		title, _ := params.GetArguments()["title"].(string)
		content, _ := params.GetArguments()["content"].(string)
		parentID, _ := params.GetArguments()["parent_id"].(string)

		pageContent := &goconfluence.Content{
			Type:  "page",
			Title: title,
			Space: &goconfluence.Space{Key: spaceKey},
			Body: goconfluence.Body{
				Storage: goconfluence.Storage{
					Value:          content,
					Representation: "storage",
				},
			},
		}

		if parentID != "" {
			pageContent.Ancestors = []goconfluence.Ancestor{
				{ID: parentID},
			}
		}

		createdPage, err := confluenceAPI.CreateContent(pageContent)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to create page: %w", err)), nil
		}

		return api.NewToolCallResult(fmt.Sprintf("Page created successfully with ID %s. View it at: %s", createdPage.ID, createdPage.Links.WebUI), nil), nil
	}
}

type disabledToolset struct{}

func (t *disabledToolset) GetName() string {
	return "confluence"
}

func (t *disabledToolset) GetDescription() string {
	return "Confluence toolset is disabled. Please configure it in the server settings."
}

func (t *disabledToolset) GetTools(_ kubernetes.Openshift) []api.ServerTool {
	return nil
}
