package kiali

import (
	"encoding/json"
	"fmt"
	"strings"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"sigs.k8s.io/yaml"
)

// Raw Kiali GET /api/namespaces/{ns}/istio/... response
type rawIstioConfigGet struct {
	GVK         *rawGVK                `json:"gvk"`
	Namespace   *rawNamespaceRef       `json:"namespace"`
	Permissions *rawPermissions        `json:"permissions"`
	References  *rawReferences         `json:"references"`
	Resource    map[string]interface{} `json:"resource"`
	Validation  *rawValidationGet      `json:"validation"`
}

type rawGVK struct {
	Group   string `json:"Group"`
	Version string `json:"Version"`
	Kind    string `json:"Kind"`
}

type rawNamespaceRef struct {
	Name string `json:"name"`
}

type rawPermissions struct {
	Create bool `json:"create"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

type rawReferences struct {
	ObjectReferences  []rawObjectRef  `json:"objectReferences"`
	ServiceReferences []rawServiceRef `json:"serviceReferences"`
}

type rawObjectRef struct {
	ObjectGVK rawGVK `json:"objectGVK"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type rawServiceRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type rawValidationGet struct {
	Valid  bool          `json:"valid"`
	Checks []rawCheckGet `json:"checks"`
}

type rawCheckGet struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
}

const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// TransformIstioConfigGet converts the raw get-Istio-object API response into IstioConfigGetFormatted.
func TransformIstioConfigGet(rawJSON string) (*kialitypes.IstioConfigGetFormatted, error) {
	var raw rawIstioConfigGet
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal istio config get: %w", err)
	}

	out := &kialitypes.IstioConfigGetFormatted{
		Summary:     kialitypes.IstioConfigGetSummary{Permissions: []string{}},
		Diagnostics: kialitypes.IstioConfigGetDiagnostics{Issues: []kialitypes.IstioConfigGetIssue{}},
		Relations:   kialitypes.IstioConfigGetRelations{Gateways: []string{}, Services: []string{}},
	}

	// summary: name, namespace, kind, status, permissions
	if raw.Resource != nil {
		if meta, _ := raw.Resource["metadata"].(map[string]interface{}); meta != nil {
			if n, _ := meta["name"].(string); n != "" {
				out.Summary.Name = n
			}
			if ns, _ := meta["namespace"].(string); ns != "" {
				out.Summary.Namespace = ns
			}
		}
	}
	if out.Summary.Namespace == "" && raw.Namespace != nil {
		out.Summary.Namespace = raw.Namespace.Name
	}
	if raw.GVK != nil {
		out.Summary.Kind = raw.GVK.Kind
	}
	if out.Summary.Kind == "" && raw.Resource != nil {
		if k, _ := raw.Resource["kind"].(string); k != "" {
			out.Summary.Kind = k
		}
	}
	out.Summary.Status = statusFromValidation(raw.Validation)
	if raw.Permissions != nil {
		if raw.Permissions.Create {
			out.Summary.Permissions = append(out.Summary.Permissions, "create")
		}
		if raw.Permissions.Update {
			out.Summary.Permissions = append(out.Summary.Permissions, "update")
		}
		if raw.Permissions.Delete {
			out.Summary.Permissions = append(out.Summary.Permissions, "delete")
		}
	}

	// diagnostics
	if raw.Validation != nil {
		out.Diagnostics.Valid = raw.Validation.Valid
		for _, c := range raw.Validation.Checks {
			out.Diagnostics.Issues = append(out.Diagnostics.Issues, kialitypes.IstioConfigGetIssue{
				Severity: strings.ToLower(strings.TrimSpace(c.Severity)),
				Code:     c.Code,
				Message:  c.Message,
				Location: c.Path,
			})
		}
	}

	// relations: gateways from objectReferences (Kind=Gateway), services from serviceReferences
	if raw.References != nil {
		for _, ref := range raw.References.ObjectReferences {
			if ref.ObjectGVK.Kind == "Gateway" && ref.Name != "" {
				out.Relations.Gateways = append(out.Relations.Gateways, ref.Name)
			}
		}
		for _, ref := range raw.References.ServiceReferences {
			if ref.Name != "" {
				out.Relations.Services = append(out.Relations.Services, ref.Name)
			}
		}
	}

	// yaml_raw: sanitized resource as YAML
	yamlRaw, err := resourceToSanitizedYAML(raw.Resource)
	if err != nil {
		return nil, fmt.Errorf("resource to yaml: %w", err)
	}
	out.YAMLRaw = yamlRaw

	return out, nil
}

func statusFromValidation(v *rawValidationGet) string {
	if v == nil {
		return "ok"
	}
	hasError := false
	hasWarning := false
	for _, c := range v.Checks {
		switch strings.ToLower(c.Severity) {
		case "error":
			hasError = true
		case "warning":
			hasWarning = true
		}
	}
	if hasError {
		return "error"
	}
	if hasWarning {
		return "warning"
	}
	return "ok"
}

// resourceToSanitizedYAML copies resource, removes status/managedFields/last-applied, marshals to YAML.
func resourceToSanitizedYAML(resource map[string]interface{}) (string, error) {
	if resource == nil {
		return "", nil
	}
	// Deep copy and sanitize
	sanitized := make(map[string]interface{})
	for k, v := range resource {
		if k == "status" {
			continue
		}
		if k == "metadata" {
			meta, ok := v.(map[string]interface{})
			if !ok {
				sanitized[k] = v
				continue
			}
			sanitized[k] = sanitizeMetadata(meta)
			continue
		}
		sanitized[k] = v
	}
	b, err := yaml.Marshal(sanitized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func sanitizeMetadata(meta map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	drop := map[string]bool{
		"managedFields":     true,
		"creationTimestamp": true,
		"resourceVersion":   true,
		"uid":               true,
		"generation":        true,
	}
	for k, v := range meta {
		if drop[k] {
			continue
		}
		if k == "annotations" {
			ann, ok := v.(map[string]interface{})
			if ok {
				cleaned := make(map[string]interface{})
				for ak, av := range ann {
					if ak == lastAppliedAnnotation {
						continue
					}
					cleaned[ak] = av
				}
				if len(cleaned) > 0 {
					out[k] = cleaned
				}
				continue
			}
		}
		out[k] = v
	}
	return out
}
