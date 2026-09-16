package api

import (
	"errors"
	"fmt"
	"slices"
)

const (
	// RBACMetadataKey is the key used for RBAC declarations in MCP _meta fields.
	RBACMetadataKey = "io.kubernetes-mcp-server/rbac"
	// RBACVersionV1Alpha1 is the initial RBAC metadata contract version.
	RBACVersionV1Alpha1 = "v1alpha1"
)

// RBACMetadata describes the Kubernetes authorization requirements of an MCP
// tool, prompt, resource, or resource template.
type RBACMetadata struct {
	Version   string         `json:"version"`
	None      *NoRBAC        `json:"none,omitempty"`
	Bounded   *BoundedRBAC   `json:"bounded,omitempty"`
	Unbounded *UnboundedRBAC `json:"unbounded,omitempty"`
}

// NoRBAC declares that a capability does not access the Kubernetes API.
type NoRBAC struct {
	Reason string `json:"reason,omitempty"`
}

// BoundedRBAC declares a finite set of Kubernetes authorization requirements.
type BoundedRBAC struct {
	Requirements []RBACRequirement `json:"requirements"`
}

// UnboundedRBAC declares that authorization requirements cannot be determined
// from the capability declaration and its arguments.
type UnboundedRBAC struct {
	Reason string `json:"reason"`
}

// RBACRequirement describes access to one Kubernetes resource target.
type RBACRequirement struct {
	Verbs        []string          `json:"verbs"`
	Target       RBACTarget        `json:"target"`
	Namespace    *RBACNamespace    `json:"namespace,omitempty"`
	ResourceName *RBACResourceName `json:"resourceName,omitempty"`
}

// RBACTarget contains exactly one resource identity representation.
type RBACTarget struct {
	Resource *RBACResourceTarget `json:"resource,omitempty"`
	GVK      *RBACGVKTarget      `json:"gvk,omitempty"`
	Manifest *RBACManifestTarget `json:"manifest,omitempty"`
}

// RBACResourceTarget identifies a Kubernetes RBAC resource directly.
type RBACResourceTarget struct {
	APIGroup    string `json:"apiGroup,omitempty"`
	Resource    string `json:"resource"`
	Subresource string `json:"subresource,omitempty"`
}

// RBACGVKTarget derives a Kubernetes resource from API version and kind arguments.
type RBACGVKTarget struct {
	APIVersionArgument string `json:"apiVersionArgument"`
	KindArgument       string `json:"kindArgument"`
	Subresource        string `json:"subresource,omitempty"`
}

// RBACManifestTarget derives resource identities from a manifest argument.
type RBACManifestTarget struct {
	Argument string `json:"argument"`
}

// RBACNamespace identifies a fixed, argument-derived, or all-namespace scope.
type RBACNamespace struct {
	Name          string `json:"name,omitempty"`
	Argument      string `json:"argument,omitempty"`
	AllNamespaces bool   `json:"all,omitempty"`
}

// RBACResourceName identifies a fixed or argument-derived resource name.
type RBACResourceName struct {
	Name     string `json:"name,omitempty"`
	Argument string `json:"argument,omitempty"`
}

// RBACNone creates metadata for a capability that does not access the Kubernetes API.
func RBACNone() *RBACMetadata {
	return &RBACMetadata{
		Version: RBACVersionV1Alpha1,
		None:    &NoRBAC{},
	}
}

// RBACBounded creates metadata with a finite set of authorization requirements.
func RBACBounded(requirements ...RBACRequirement) *RBACMetadata {
	return &RBACMetadata{
		Version: RBACVersionV1Alpha1,
		Bounded: &BoundedRBAC{Requirements: requirements},
	}
}

// RBACUnbounded creates metadata for requirements that cannot be derived in advance.
func RBACUnbounded(reason string) *RBACMetadata {
	return &RBACMetadata{
		Version:   RBACVersionV1Alpha1,
		Unbounded: &UnboundedRBAC{Reason: reason},
	}
}

// Validate verifies that the metadata contains one valid declaration form.
func (r *RBACMetadata) Validate() error {
	if r == nil {
		return errors.New("RBAC metadata is nil")
	}
	if r.Version != RBACVersionV1Alpha1 {
		return fmt.Errorf("unsupported RBAC metadata version %q", r.Version)
	}

	declaredRbacForms := 0
	if r.None != nil {
		declaredRbacForms++
	}
	if r.Bounded != nil {
		declaredRbacForms++
	}
	if r.Unbounded != nil {
		declaredRbacForms++
	}
	if declaredRbacForms != 1 {
		return errors.New("RBAC metadata must contain exactly one of none, bounded, or unbounded")
	}

	if r.Bounded != nil {
		return r.Bounded.validate()
	}
	if r.Unbounded != nil && r.Unbounded.Reason == "" {
		return errors.New("unbounded RBAC metadata must include a reason")
	}

	return nil
}

func (r *BoundedRBAC) validate() error {
	if len(r.Requirements) == 0 {
		return errors.New("bounded RBAC metadata must include at least one requirement")
	}

	for i := range r.Requirements {
		if err := r.Requirements[i].validate(); err != nil {
			return fmt.Errorf("requirement %d: %w", i, err)
		}
	}

	return nil
}

func (r *RBACRequirement) validate() error {
	if len(r.Verbs) == 0 {
		return errors.New("verbs must not be empty")
	}

	if slices.Contains(r.Verbs, "") {
		return errors.New("verbs must not contain an empty value")
	}

	if err := r.Target.validate(); err != nil {
		return err
	}

	if r.Namespace != nil {
		if err := r.Namespace.validate(); err != nil {
			return err
		}
	}

	if r.ResourceName != nil {
		if err := r.ResourceName.validate(); err != nil {
			return err
		}
	}

	return nil
}

func (t *RBACTarget) validate() error {
	declaredTargetForms := 0
	if t.Resource != nil {
		declaredTargetForms++
	}
	if t.GVK != nil {
		declaredTargetForms++
	}
	if t.Manifest != nil {
		declaredTargetForms++
	}
	if declaredTargetForms != 1 {
		return errors.New("target must contain exactly one of resource, gvk, or manifest")
	}

	if t.Resource != nil {
		return t.Resource.validate()
	}
	if t.GVK != nil {
		return t.GVK.validate()
	}
	if t.Manifest.Argument == "" {
		return errors.New("manifest target argument must not be empty")
	}

	return nil
}

func (t *RBACResourceTarget) validate() error {
	if t.Resource == "" {
		return errors.New("resource target resource must not be empty")
	}

	return nil
}

func (t *RBACGVKTarget) validate() error {
	if t.APIVersionArgument == "" {
		return errors.New("gvk target apiVersionArgument must not be empty")
	}

	if t.KindArgument == "" {
		return errors.New("gvk target kindArgument must not be empty")
	}

	return nil
}

func (n *RBACNamespace) validate() error {
	declaredNamespaceForms := 0
	if n.Name != "" {
		declaredNamespaceForms++
	}
	if n.Argument != "" {
		declaredNamespaceForms++
	}
	if n.AllNamespaces {
		declaredNamespaceForms++
	}
	if declaredNamespaceForms != 1 {
		return errors.New("namespace must contain exactly one of name, argument, or all")
	}

	return nil
}

func (n *RBACResourceName) validate() error {
	if (n.Name == "") == (n.Argument == "") {
		return errors.New("resourceName must contain exactly one of name or argument")
	}

	return nil
}
