##@ Istio/Kiali

ISTIOCTL = $(shell pwd)/_output/tools/bin/istioctl
ISTIO_ADDONS_DIR = $(shell pwd)/_output/istio-addons
ISTIO_VERSION = 1.31.0
KIALI_VERSION = v2.33
# Release version without patch (e.g. 1.28.0 -> 1.28)

# Download and install istioctl (also copies samples/addons for install-istio)
.PHONY: istioctl
istioctl:
	@{ \
		set -e ;\
		echo "Installing istioctl to $(ISTIOCTL)..." ;\
		mkdir -p $(shell dirname $(ISTIOCTL)) ;\
		TMPDIR=$$(mktemp -d) ;\
		cd $$TMPDIR ;\
		curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$(ISTIO_VERSION) sh - ; \
		ISTIODIR=$$(ls -d istio-* | head -n1) ;\
		cp $$ISTIODIR/bin/istioctl $(ISTIOCTL) ;\
		mkdir -p $(ISTIO_ADDONS_DIR) ;\
		cp $$ISTIODIR/samples/addons/jaeger.yaml $$ISTIODIR/samples/addons/prometheus.yaml $$ISTIODIR/samples/addons/kiali.yaml $(ISTIO_ADDONS_DIR)/ ;\
		sed -i '/ tracing:/,/ identity:/ { s/ enabled: false/ enabled: true\n        in_cluster_url: "http:\/\/tracing.istio-system:16685\/jaeger"\n        use_grpc: true/ }' $(ISTIO_ADDONS_DIR)/kiali.yaml ;\
		cd - >/dev/null ;\
		rm -rf $$TMPDIR ;\
	}

# Install Gateway API CRDs required for Gateway API eval tasks and Kiali AI tools
.PHONY: install-gateway-api-crds
install-gateway-api-crds:
	@evals/tasks/kiali/scripts/ensure_gateway_api_crds.sh

# Install Istio (demo profile) and enable sidecar injection in default namespace
.PHONY: install-istio
install-istio: istioctl kubectl
	$(ISTIOCTL) install --set profile=demo \
		--set meshConfig.defaultConfig.tracing.zipkin.address=zipkin.istio-system:9411 \
		-y
	$(KUBECTL) apply -f $(ISTIO_ADDONS_DIR)/prometheus.yaml -n istio-system
	$(KUBECTL) apply -f $(ISTIO_ADDONS_DIR)/kiali.yaml -n istio-system
	$(KUBECTL) apply -f $(ISTIO_ADDONS_DIR)/jaeger.yaml -n istio-system
	$(KUBECTL) wait --namespace istio-system --for=condition=available deployment/kiali --timeout=300s
	$(KUBECTL) wait --namespace istio-system --for=condition=available deployment/prometheus --timeout=300s
	$(KUBECTL) wait --for=condition=Ready pod --all -n istio-system --timeout=300s
	$(KUBECTL) rollout status deployment/kiali -n istio-system
	$(KUBECTL) label namespace default istio-injection=enabled --overwrite
	$(KUBECTL) wait --for=condition=Ready pod --all -n istio-system --timeout=300s
	
# Install Bookinfo demo
.PHONY: install-bookinfo-demo
install-bookinfo-demo: kubectl
	$(KUBECTL) create ns bookinfo
	$(KUBECTL) label namespace bookinfo istio-discovery=enabled istio.io/rev=default istio-injection=enabled
	$(KUBECTL) apply -f https://raw.githubusercontent.com/openshift-service-mesh/istio/refs/heads/master/samples/bookinfo/platform/kube/bookinfo.yaml -n bookinfo
	$(KUBECTL) apply -n bookinfo -f https://raw.githubusercontent.com/istio-ecosystem/sail-operator/main/chart/samples/ingress-gateway.yaml
	$(KUBECTL) apply -f https://raw.githubusercontent.com/openshift-service-mesh/istio/refs/heads/master/samples/bookinfo/networking/bookinfo-gateway.yaml -n bookinfo
	$(KUBECTL) wait --for=condition=Ready pod --all -n bookinfo --timeout=300s

# Update Kiali version
.PHONY: update-kiali-version
update-kiali-version: kubectl
	@echo "Updating Kiali version to $(KIALI_VERSION)..."
	@$(KUBECTL) patch deployment kiali -n istio-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"kiali","image":"quay.io/kiali/kiali_mcp:$(KIALI_VERSION)"}]}}}}'
	@$(KUBECTL) delete pod -l app=kiali -n istio-system
	@$(KUBECTL) wait --for=condition=available deployment/kiali -n istio-system --timeout=300s

# Expose Bookinfo demo
.PHONY: expose-bookinfo-demo
expose-bookinfo-demo: kubectl
	@echo "Exposing Bookinfo demo..."
	@$(KUBECTL) port-forward svc/istio-ingressgateway 20002:80 -n bookinfo >/dev/null 2>&1 & \
	while true; do curl -s -o /dev/null http://localhost:20002/productpage; sleep 1; done & \
	echo "Bookinfo demo is being exposed on http://localhost:20002/productpage and generator is running"

# Expose Kiali service
.PHONY: expose-kiali
expose-kiali: kubectl
	@echo "Exposing Kiali service..."
	$(KUBECTL) -n istio-system port-forward svc/kiali 20001:20001 & \
	timeout 30s bash -c 'until curl -s localhost:20001; do sleep 1; done' && \
	echo "Kiali is being exposed on http://localhost:20001"

.PHONY: setup-kiali
setup-kiali: install-istio install-gateway-api-crds update-kiali-version install-bookinfo-demo expose-kiali expose-bookinfo-demo ## Setup Kiali

define UPDATE_KIALI_DOCS_PY
import json
from pathlib import Path

schema = Path("pkg/mcp/testdata/toolsets-kiali-tools.json")
doc = Path("docs/KIALI.md")
start_mark = "<!-- KIALI-TOOLS:START -->"
end_mark = "<!-- KIALI-TOOLS:END -->"
h = chr(35)

def cell(value):
    return str(value).replace("|", "\\|").replace("\n", " ")

def param_type(prop):
    kind = prop.get("type", "")
    enum = prop.get("enum") or []
    if not enum:
        return kind
    vals = ", ".join("`" + str(item) + "`" for item in enum)
    return kind + " (" + vals + ")"

def fmt_default(value):
    if isinstance(value, bool):
        return "true" if value else "false"
    return str(value)

def describe(prop):
    text = prop.get("description", "")
    if "minimum" in prop:
        text = text.rstrip(".") + ". Minimum: " + fmt_default(prop["minimum"]) + "."
    if "maximum" in prop:
        text = text.rstrip(".") + ". Maximum: " + fmt_default(prop["maximum"]) + "."
    return cell(text)

def render(tools):
    tools = sorted(tools, key=lambda tool: tool["name"])
    lines = [
        "Generated from [`pkg/mcp/testdata/toolsets-kiali-tools.json`](../pkg/mcp/testdata/toolsets-kiali-tools.json). Refresh this list with `make update-kiali-docs`.",
        "",
        "| Tool | Access | Purpose |",
        "| --- | --- | --- |",
    ]
    for tool in tools:
        name = tool["name"]
        ann = tool.get("annotations") or {}
        access = "Read-only" if ann.get("readOnlyHint") else "Writes config"
        title = ann.get("title") or tool.get("title") or name
        lines.append("| [`" + name + "`](" + h + name + ") | " + access + " | " + cell(title) + " |")
    lines.append("")
    for tool in tools:
        name = tool["name"]
        ann = tool.get("annotations") or {}
        access = "Read-only" if ann.get("readOnlyHint") else "Writes config"
        title = ann.get("title") or tool.get("title") or name
        lines.append(h * 4 + " `" + name + "`")
        lines.append("")
        lines.append("**" + title + ".** " + access + ".")
        lines.append("")
        lines.append((tool.get("description") or "").strip())
        lines.append("")
        schema_obj = tool.get("inputSchema") or {}
        props = schema_obj.get("properties") or {}
        required = set(schema_obj.get("required") or [])
        if not props:
            lines.append("This tool takes no parameters.")
            lines.append("")
            continue
        lines.append("| Parameter | Type | Required | Default | Description |")
        lines.append("| --- | --- | --- | --- | --- |")
        for pname in sorted(props, key=lambda n: (n not in required, n)):
            prop = props[pname]
            default = "`" + fmt_default(prop["default"]) + "`" if "default" in prop else ""
            req = "yes" if pname in required else "no"
            lines.append("| `" + pname + "` | " + param_type(prop) + " | " + req + " | " + default + " | " + describe(prop) + " |")
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"

text = doc.read_text()
try:
    start = text.index(start_mark) + len(start_mark)
    end = text.index(end_mark)
except ValueError:
    raise SystemExit("docs/KIALI.md is missing the KIALI-TOOLS markers")
if start > end:
    raise SystemExit("KIALI-TOOLS markers are out of order")
doc.write_text(text[:start] + "\n" + render(json.loads(schema.read_text())) + text[end:])
print("Updated docs/KIALI.md")
endef
export UPDATE_KIALI_DOCS_PY

.PHONY: update-kiali-docs
update-kiali-docs: ## Regenerate the Kiali tool catalog in docs/KIALI.md
	@python3 -c 'import os; exec(os.environ["UPDATE_KIALI_DOCS_PY"])'
