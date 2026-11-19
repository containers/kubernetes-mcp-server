# CSI hostPath driver installation (resize-capable StorageClass for evals)
#
# Provides the `csi-hostpath-sc` StorageClass with allowVolumeExpansion: true so
# the core `resize-pvc` eval task can run. Minikube's default `standard` StorageClass
# does not support volume expansion.

# Reference-only: the upstream release the vendored manifest was trimmed from
# (kubernetes-csi/csi-driver-host-path). Used only in the log line below; the
# actual image tags are pinned inside the manifest, so bumping this alone changes
# nothing — update dev/config/csi-hostpath/csi-hostpath-driver.yaml to upgrade.
CSI_HOSTPATH_VERSION ?= v1.17.1
CSI_HOSTPATH_MANIFEST = dev/config/csi-hostpath/csi-hostpath-driver.yaml

##@ Storage

.PHONY: csi-hostpath-install
csi-hostpath-install: kubectl ## Install a resize-capable hostPath CSI StorageClass (csi-hostpath-sc)
	@echo "========================================="
	@echo "Installing hostPath CSI driver (trimmed from $(CSI_HOSTPATH_VERSION))"
	@echo "========================================="
	@echo ""
	@echo "Applying CSI hostPath driver manifest..."
	@$(KUBECTL) apply -f $(CSI_HOSTPATH_MANIFEST)
	@echo ""
	@echo "Waiting for the CSI hostPath plugin to be ready..."
	@$(KUBECTL) rollout status statefulset/csi-hostpathplugin -n csi-driver --timeout=5m
	@echo "✅ CSI hostPath driver ready (StorageClass: csi-hostpath-sc)"

.PHONY: csi-hostpath-uninstall
csi-hostpath-uninstall: kubectl ## Uninstall the hostPath CSI driver
	@echo "Uninstalling hostPath CSI driver..."
	@$(KUBECTL) delete -f $(CSI_HOSTPATH_MANIFEST) --ignore-not-found
	@echo "✅ CSI hostPath driver uninstalled"

.PHONY: csi-hostpath-status
csi-hostpath-status: kubectl ## Show hostPath CSI driver status
	@echo "CSI hostPath pods:"
	@$(KUBECTL) get pods -n csi-driver 2>/dev/null || echo "CSI hostPath driver not installed"
	@echo ""
	@echo "StorageClasses:"
	@$(KUBECTL) get storageclass 2>/dev/null || true
