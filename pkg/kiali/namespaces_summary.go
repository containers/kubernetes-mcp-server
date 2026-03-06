package kiali

import (
	"fmt"
	"sort"
	"strings"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// lowSuccessRateThreshold is the success rate below which we add a "Low success rate detected" note (e.g. 0.70 = 70%).
const lowSuccessRateThreshold = 0.70

func buildNamespaceSummary(ns kialitypes.NamespaceListItemRaw, health kialitypes.ClustersNamespaceHealth) kialitypes.NamespaceSummaryFormatted {
	appsHealth := health.AppHealth[ns.Name]
	appNames := make([]string, 0, len(appsHealth))
	for name := range appsHealth {
		appNames = append(appNames, name)
	}
	sort.Strings(appNames)

	apps := make([]kialitypes.AppSummaryFormatted, 0, len(appNames))
	var healthy, degraded, unhealthy, notReady int
	for _, appName := range appNames {
		app := appsHealth[appName]
		sum := buildAppSummary(appName, app, ns.IsControlPlane)
		apps = append(apps, sum)
		switch sum.Status {
		case "Healthy":
			healthy++
		case "Degraded":
			degraded++
		case "Unhealthy":
			unhealthy++
		case "Not Ready":
			notReady++
		default:
			healthy++
		}
	}

	summary := buildNamespaceSummaryLine(len(apps), healthy, degraded, unhealthy, notReady, ns.IsControlPlane)
	return kialitypes.NamespaceSummaryFormatted{
		Name:    ns.Name,
		Summary: summary,
		Apps:    apps,
	}
}

func buildAppSummary(appName string, app kialitypes.AppHealth, isControlPlane bool) kialitypes.AppSummaryFormatted {
	status, issue := evaluateAppHealth(app)
	statusHuman := healthStatusToHuman(status)

	successRate := ""
	errorRate := calculateErrorRate(app.Requests)
	if errorRate >= 0 && errorRate <= 1 {
		successPct := (1 - errorRate) * 100
		successRate = fmt.Sprintf("%.1f%%", successPct)
	}

	replicas := ""
	var proxiesSynced *bool
	if len(app.WorkloadStatuses) > 0 {
		var totalDesired, totalCurrent int32
		allSynced := true
		for _, ws := range app.WorkloadStatuses {
			totalDesired += ws.DesiredReplicas
			totalCurrent += ws.CurrentReplicas
			if ws.SyncedProxies >= 0 && ws.AvailableReplicas > 0 && ws.SyncedProxies < ws.AvailableReplicas {
				allSynced = false
			}
		}
		replicas = fmt.Sprintf("%d/%d", totalCurrent, totalDesired)
		proxiesSynced = &allSynced
	}

	notes := issue
	if successRate != "" {
		if (1 - errorRate) < lowSuccessRateThreshold {
			if notes != "" {
				notes = notes + "; Low success rate detected"
			} else {
				notes = "Low success rate detected"
			}
		}
	}

	out := kialitypes.AppSummaryFormatted{
		Name:   appName,
		Status: statusHuman,
		Notes:  notes,
	}
	if successRate != "" {
		out.SuccessRate = successRate
	}
	if replicas != "" {
		out.Replicas = replicas
	}
	if proxiesSynced != nil && !isControlPlane {
		out.ProxiesSynced = proxiesSynced
	}
	// For control plane, omit success_rate and proxies_synced to match "Infrastructure" style
	if isControlPlane {
		out.SuccessRate = ""
		out.ProxiesSynced = nil
	}
	return out
}

func healthStatusToHuman(s string) string {
	switch s {
	case "HEALTHY":
		return "Healthy"
	case "DEGRADED":
		return "Degraded"
	case "UNHEALTHY":
		return "Unhealthy"
	case "NOT_READY":
		return "Not Ready"
	default:
		return "Unknown"
	}
}

func buildNamespaceSummaryLine(numApps, healthy, degraded, unhealthy, notReady int, isControlPlane bool) string {
	if numApps == 0 {
		return "0 apps"
	}
	appLabel := "apps"
	if numApps == 1 {
		appLabel = "app"
	}
	infra := ""
	if isControlPlane {
		infra = " (Infrastructure)"
	}
	part := fmt.Sprintf("%d %s%s", numApps, appLabel, infra)
	if unhealthy > 0 {
		return part + ", " + fmt.Sprintf("%d Unhealthy", unhealthy)
	}
	if degraded > 0 {
		return part + ", " + fmt.Sprintf("%d Degraded", degraded)
	}
	if notReady > 0 {
		return part + ", " + fmt.Sprintf("%d Not Ready", notReady)
	}
	return part + ", All Healthy"
}

// formatSuccessRate formats a 0-1 success rate as a percentage string.
func formatSuccessRate(rate float64) string {
	if rate <= 0 {
		return "0%"
	}
	if rate >= 1 {
		return "100%"
	}
	return fmt.Sprintf("%.1f%%", rate*100)
}

// --- Service health overview (type=service) ---

func TransformNamespacesToTypeHealth(HealthType string, namespaces []kialitypes.NamespaceListItemRaw, health kialitypes.ClustersNamespaceHealth) (interface{}, error) {
	nsList := namespaces
	// If no namespaces list, derive from workload health keys
	if len(namespaces) == 0 {
		for nsName := range health.WorkloadHealth {
			namespaces = append(nsList, kialitypes.NamespaceListItemRaw{Name: nsName})
		}
		sort.Slice(nsList, func(i, j int) bool { return nsList[i].Name < nsList[j].Name })
	}
	switch HealthType {
	case "service":
		{
			out := &kialitypes.ServiceHealthOverviewResponse{
				ServiceHealthOverview: make([]kialitypes.ServiceNamespaceOverview, 0, len(namespaces)),
			}
			for _, ns := range namespaces {
				entry := buildServiceNamespaceOverview(ns, health)
				out.ServiceHealthOverview = append(out.ServiceHealthOverview, entry)
			}
			return out, nil
		}
	case "workload":
		{
			out := &kialitypes.WorkloadHealthOverviewResponse{
				WorkloadHealth: make([]kialitypes.WorkloadNamespaceOverview, 0, len(nsList)),
			}
			for _, ns := range nsList {
				entry := buildWorkloadNamespaceOverview(ns, health)
				out.WorkloadHealth = append(out.WorkloadHealth, entry)
			}
			return out, nil
		}
	case "app":
		{
			out := &kialitypes.NamespacesSummaryResponse{
				Namespaces: make([]kialitypes.NamespaceSummaryFormatted, 0, len(namespaces)),
			}
			for _, ns := range namespaces {
				entry := buildNamespaceSummary(ns, health)
				out.Namespaces = append(out.Namespaces, entry)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("invalid health type: %s", HealthType)
}

func buildServiceNamespaceOverview(ns kialitypes.NamespaceListItemRaw, health kialitypes.ClustersNamespaceHealth) kialitypes.ServiceNamespaceOverview {
	if ns.IsControlPlane {
		return kialitypes.ServiceNamespaceOverview{
			Namespace: ns.Name,
			Summary:   "Infrastructure services (istiod, ingress, etc.) are NA/Healthy",
			Status:    "Operational",
		}
	}
	svcs := health.ServiceHealth[ns.Name]
	if len(svcs) == 0 {
		return kialitypes.ServiceNamespaceOverview{Namespace: ns.Name}
	}
	names := make([]string, 0, len(svcs))
	for n := range svcs {
		names = append(names, n)
	}
	sort.Strings(names)
	var degraded []kialitypes.ServiceDegradedEntry
	var active []kialitypes.ServiceActiveEntry
	healthyCount := 0
	for _, name := range names {
		svc := svcs[name]
		status, issue := evaluateServiceHealth(svc)
		statusHuman := healthStatusToHuman(status)
		errorRate := calculateErrorRate(svc.Requests)
		inboundErr := calculateInboundErrorRate(svc.Requests)
		successRate := 1 - errorRate
		inboundSuccess := 1 - inboundErr
		successStr := formatSuccessRate(successRate)
		inboundStr := formatSuccessRate(inboundSuccess)
		if status != "HEALTHY" || successRate < lowSuccessRateThreshold {
			note := issue
			if note == "" && successRate < lowSuccessRateThreshold {
				note = "Low success rate"
			}
			degraded = append(degraded, kialitypes.ServiceDegradedEntry{
				Name:               name,
				InboundSuccessRate: inboundStr,
				Status:             statusHuman,
				Issue:              note,
			})
		} else {
			healthyCount++
			active = append(active, kialitypes.ServiceActiveEntry{Name: name, SuccessRate: successStr})
		}
	}
	return kialitypes.ServiceNamespaceOverview{
		Namespace:        ns.Name,
		HealthyCount:     healthyCount,
		DegradedServices: degraded,
		ActiveServices:   active,
	}
}

// severeTrafficThreshold is the inbound success rate below which we label "Severe traffic degradation".
const severeTrafficThreshold = 0.50

func buildWorkloadNamespaceOverview(ns kialitypes.NamespaceListItemRaw, health kialitypes.ClustersNamespaceHealth) kialitypes.WorkloadNamespaceOverview {
	wls := health.WorkloadHealth[ns.Name]
	if len(wls) == 0 {
		return kialitypes.WorkloadNamespaceOverview{Namespace: ns.Name}
	}
	names := make([]string, 0, len(wls))
	for n := range wls {
		names = append(names, n)
	}
	sort.Strings(names)
	var critical []kialitypes.WorkloadCriticalIssue
	var stable []kialitypes.WorkloadStableEntry
	for _, name := range names {
		wl := wls[name]
		status, issue := evaluateWorkloadHealth(wl)
		statusHuman := healthStatusToHuman(status)
		errorRate := calculateErrorRate(wl.Requests)
		inboundErr := calculateInboundErrorRate(wl.Requests)
		successRate := 1 - errorRate
		inboundSuccess := 1 - inboundErr
		successStr := formatSuccessRate(successRate)
		inboundStr := formatSuccessRate(inboundSuccess)
		replicas := ""
		proxies := ""
		if wl.WorkloadStatus != nil {
			ws := wl.WorkloadStatus
			replicas = fmt.Sprintf("%d/%d", ws.CurrentReplicas, ws.DesiredReplicas)
			if ws.SyncedProxies < 0 {
				proxies = "N/A"
			} else {
				proxies = fmt.Sprintf("%d", ws.SyncedProxies)
			}
		}
		if successRate < lowSuccessRateThreshold || status != "HEALTHY" {
			note := issue
			if note == "" && successRate < lowSuccessRateThreshold {
				note = "Low success rate"
			}
			if inboundSuccess < severeTrafficThreshold && note == "Low success rate" {
				note = "Severe traffic degradation"
			}
			statusNote := statusHuman
			if wl.WorkloadStatus != nil && wl.WorkloadStatus.AvailableReplicas == wl.WorkloadStatus.DesiredReplicas && status == "HEALTHY" {
				statusNote = "Healthy (Replicas OK)"
			}
			critical = append(critical, kialitypes.WorkloadCriticalIssue{
				Workload:       name,
				Issue:          note,
				InboundSuccess: inboundStr,
				Status:         statusNote,
			})
		} else {
			ent := kialitypes.WorkloadStableEntry{Name: name, Success: successStr}
			if replicas != "" {
				ent.Replicas = replicas
			}
			if proxies != "" {
				ent.Proxies = proxies
			}
			stable = append(stable, ent)
		}
	}
	// Group critical by app prefix for "reviews (v1, v2, v3)" style
	critical = groupWorkloadCriticalByApp(critical)
	if ns.IsControlPlane {
		wlNames := append([]string(nil), names...)
		sort.Strings(wlNames)
		return kialitypes.WorkloadNamespaceOverview{
			Namespace: ns.Name,
			Status:    "All Infrastructure Healthy",
			Workloads: wlNames,
		}
	}
	return kialitypes.WorkloadNamespaceOverview{
		Namespace:       ns.Name,
		CriticalIssues:  critical,
		StableWorkloads: stable,
	}
}

// groupWorkloadCriticalByApp groups workloads like reviews-v1, reviews-v2, reviews-v3 into "reviews (v1, v2, v3)".
func groupWorkloadCriticalByApp(issues []kialitypes.WorkloadCriticalIssue) []kialitypes.WorkloadCriticalIssue {
	if len(issues) == 0 {
		return issues
	}
	byBase := make(map[string][]kialitypes.WorkloadCriticalIssue)
	for _, i := range issues {
		idx := strings.LastIndex(i.Workload, "-")
		if idx > 0 {
			base := i.Workload[:idx]
			byBase[base] = append(byBase[base], i)
		} else {
			byBase[i.Workload] = append(byBase[i.Workload], i)
		}
	}
	var out []kialitypes.WorkloadCriticalIssue
	for base, list := range byBase {
		if len(list) == 1 {
			out = append(out, list[0])
			continue
		}
		// Multiple versions: "reviews (v1, v2, v3)"
		parts := make([]string, 0, len(list))
		for _, i := range list {
			suffix := i.Workload
			if strings.HasPrefix(i.Workload, base+"-") {
				suffix = i.Workload[len(base)+1:]
			}
			parts = append(parts, suffix)
		}
		sort.Strings(parts)
		combined := base + " (" + strings.Join(parts, ", ") + ")"
		out = append(out, kialitypes.WorkloadCriticalIssue{
			Workload:       combined,
			Issue:          list[0].Issue,
			InboundSuccess: list[0].InboundSuccess,
			Status:         list[0].Status,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Workload < out[j].Workload })
	return out
}
