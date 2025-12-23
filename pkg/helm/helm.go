package helm

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type Kubernetes interface {
	genericclioptions.RESTClientGetter
	NamespaceOrDefault(namespace string) string
}

type Helm struct {
	kubernetes Kubernetes
}

// NewHelm creates a new Helm instance
func NewHelm(kubernetes Kubernetes) *Helm {
	return &Helm{kubernetes: kubernetes}
}

func (h *Helm) Install(ctx context.Context, chart string, values map[string]interface{}, name string, namespace string) (string, error) {
	cfg, err := h.newAction(h.kubernetes.NamespaceOrDefault(namespace), false)
	if err != nil {
		return "", err
	}
	install := action.NewInstall(cfg)
	if name == "" {
		install.GenerateName = true
		install.ReleaseName, _, _ = install.NameAndChart([]string{chart})
	} else {
		install.ReleaseName = name
	}
	install.Namespace = h.kubernetes.NamespaceOrDefault(namespace)
	install.Wait = true
	install.Timeout = 5 * time.Minute
	install.DryRun = false

	chartRequested, err := install.LocateChart(chart, cli.New())
	if err != nil {
		return "", err
	}
	chartLoaded, err := loader.Load(chartRequested)
	if err != nil {
		return "", err
	}

	installedRelease, err := install.RunWithContext(ctx, chartLoaded, values)
	if err != nil {
		return "", err
	}
	ret, err := yaml.Marshal(simplify(installedRelease))
	if err != nil {
		return "", err
	}
	return string(ret), nil
}

// List lists all the releases for the specified namespace (or current namespace if). Or allNamespaces is true, it lists all releases across all namespaces.
func (h *Helm) List(namespace string, allNamespaces bool) (string, error) {
	cfg, err := h.newAction(namespace, allNamespaces)
	if err != nil {
		return "", err
	}
	list := action.NewList(cfg)
	list.AllNamespaces = allNamespaces
	releases, err := list.Run()
	if err != nil {
		return "", err
	} else if len(releases) == 0 {
		return "No Helm releases found", nil
	}
	ret, err := yaml.Marshal(simplify(releases...))
	if err != nil {
		return "", err
	}
	return string(ret), nil
}

func (h *Helm) Uninstall(name string, namespace string) (string, error) {
	cfg, err := h.newAction(h.kubernetes.NamespaceOrDefault(namespace), false)
	if err != nil {
		return "", err
	}
	uninstall := action.NewUninstall(cfg)
	uninstall.IgnoreNotFound = true
	uninstall.Wait = true
	uninstall.Timeout = 5 * time.Minute
	uninstalledRelease, err := uninstall.Run(name)
	if uninstalledRelease == nil && err == nil {
		return fmt.Sprintf("Release %s not found", name), nil
	} else if err != nil {
		return "", err
	}
	return fmt.Sprintf("Uninstalled release %s %s", uninstalledRelease.Release.Name, uninstalledRelease.Info), nil
}

// History retrieves the revision history for a given Helm release
func (h *Helm) History(name string, namespace string, max int) (string, error) {
	cfg, err := h.newAction(h.kubernetes.NamespaceOrDefault(namespace), false)
	if err != nil {
		return "", err
	}
	history := action.NewHistory(cfg)
	releases, err := history.Run(name)
	if err != nil {
		return "", err
	}
	if len(releases) == 0 {
		return fmt.Sprintf("No history found for release %s", name), nil
	}
	if max > 0 && len(releases) > max {
		releases = releases[len(releases)-max:]
	}
	ret, err := yaml.Marshal(simplifyHistory(releases...))
	if err != nil {
		return "", err
	}
	return string(ret), nil
}

func (h *Helm) newAction(namespace string, allNamespaces bool) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	applicableNamespace := ""
	if !allNamespaces {
		applicableNamespace = h.kubernetes.NamespaceOrDefault(namespace)
	}
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, err
	}
	cfg.RegistryClient = registryClient
	return cfg, cfg.Init(h.kubernetes, applicableNamespace, "", klog.V(5).Infof)
}

func simplify(release ...*release.Release) []map[string]interface{} {
	ret := make([]map[string]interface{}, len(release))
	for i, r := range release {
		ret[i] = map[string]interface{}{
			"name":      r.Name,
			"namespace": r.Namespace,
			"revision":  r.Version,
		}
		if r.Chart != nil {
			ret[i]["chart"] = r.Chart.Metadata.Name
			ret[i]["chartVersion"] = r.Chart.Metadata.Version
			ret[i]["appVersion"] = r.Chart.Metadata.AppVersion
		}
		if r.Info != nil {
			ret[i]["status"] = r.Info.Status.String()
			if !r.Info.LastDeployed.IsZero() {
				ret[i]["lastDeployed"] = r.Info.LastDeployed.Format(time.RFC1123Z)
			}
		}
	}
	return ret
}

func simplifyHistory(releases ...*release.Release) []map[string]interface{} {
	ret := make([]map[string]interface{}, len(releases))
	for i, r := range releases {
		ret[i] = map[string]interface{}{
			"revision":   r.Version,
			"updated":    r.Info.LastDeployed.Format(time.RFC1123Z),
			"status":     r.Info.Status.String(),
			"chart":      fmt.Sprintf("%s-%s", r.Chart.Metadata.Name, r.Chart.Metadata.Version),
			"appVersion": r.Chart.Metadata.AppVersion,
		}
		if r.Info.Description != "" {
			ret[i]["description"] = r.Info.Description
		}
	}
	return ret
}
