package troubleshoot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

type AnalysisSuite struct {
	suite.Suite
}

func TestAnalysisSuite(t *testing.T) {
	suite.Run(t, new(AnalysisSuite))
}

func (s *AnalysisSuite) TestCheckVMConditions() {
	s.Run("detects VMINotExists condition", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "VMINotExists",
						"message": "VMI does not exist",
					},
				},
			},
		})

		issues := checkVMConditions(vm)
		s.Require().NotEmpty(issues)
		s.Contains(issues[0].Message, "VMINotExists")
		s.Equal(severityWarning, issues[0].Severity)
	})

	s.Run("detects Provisioning printableStatus", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"printableStatus": "Provisioning",
			},
		})

		issues := checkVMConditions(vm)
		s.Require().NotEmpty(issues)
		found := false
		for _, iss := range issues {
			if iss.Severity == severityWarning && strings.Contains(iss.Message, "Provisioning") {
				found = true
				break
			}
		}
		s.True(found, "expected Provisioning issue")
	})

	s.Run("detects CrashLoopBackOff printableStatus", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"printableStatus": "CrashLoopBackOff",
			},
		})

		issues := checkVMConditions(vm)
		s.Require().NotEmpty(issues)
		s.Equal(severityCritical, issues[0].Severity)
		s.Contains(issues[0].Message, "CrashLoopBackOff")
		s.NotEmpty(issues[0].Fix)
	})

	s.Run("returns nil for healthy VM", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"printableStatus": "Running",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		})

		issues := checkVMConditions(vm)
		s.Empty(issues)
	})

	s.Run("returns nil for nil VM", func() {
		issues := checkVMConditions(nil)
		s.Nil(issues)
	})
}

func (s *AnalysisSuite) TestCheckDataVolumeErrors() {
	s.Run("detects PVC not found in volume status", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"volumeSnapshotStatuses": []interface{}{
					map[string]interface{}{
						"name":   "rootdisk",
						"reason": "PVC not found",
					},
				},
			},
		})

		issues := checkDataVolumeErrors(vm, nil)
		s.Require().Len(issues, 1)
		s.Equal(severityCritical, issues[0].Severity)
		s.Contains(issues[0].Message, "rootdisk")
		s.Contains(issues[0].Message, "PVC not found")
	})

	s.Run("suppresses PVC error when StorageClass is already missing", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"volumeSnapshotStatuses": []interface{}{
					map[string]interface{}{
						"name":   "rootdisk",
						"reason": "PVC not found",
					},
				},
			},
		})

		scIssues := []issue{{
			Severity: severityCritical,
			Message:  "StorageClass \"premium-nvme\" does not exist",
		}}

		issues := checkDataVolumeErrors(vm, scIssues)
		s.Empty(issues, "PVC not found should be suppressed when SC is missing (it's a symptom)")
	})

	s.Run("returns nil when volumes are healthy", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"volumeSnapshotStatuses": []interface{}{
					map[string]interface{}{
						"name":   "rootdisk",
						"reason": "",
					},
				},
			},
		})

		issues := checkDataVolumeErrors(vm, nil)
		s.Empty(issues)
	})
}

func (s *AnalysisSuite) TestCheckStorageClass() {
	ctx := context.Background()

	s.Run("detects missing StorageClass", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"dataVolumeTemplates": []interface{}{
					map[string]interface{}{
						"metadata": map[string]interface{}{"name": "test-dv"},
						"spec": map[string]interface{}{
							"storage": map[string]interface{}{
								"storageClassName": "premium-nvme-storage",
							},
						},
					},
				},
			},
		})

		existingSC := &unstructured.Unstructured{}
		existingSC.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1",
			"kind":       "StorageClass",
			"metadata": map[string]interface{}{
				"name": "standard-csi",
				"annotations": map[string]interface{}{
					"storageclass.kubernetes.io/is-default-class": "true",
				},
			},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.StorageClassGVR: "StorageClassList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, existingSC)

		issues := checkStorageClass(ctx, client, vm)
		s.Require().Len(issues, 1)
		s.Equal(severityCritical, issues[0].Severity)
		s.Contains(issues[0].Message, "premium-nvme-storage")
		s.Contains(issues[0].Message, "does not exist")
		s.Contains(issues[0].Message, "standard-csi (default)")
	})

	s.Run("returns nil when StorageClass exists", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"dataVolumeTemplates": []interface{}{
					map[string]interface{}{
						"metadata": map[string]interface{}{"name": "test-dv"},
						"spec": map[string]interface{}{
							"storage": map[string]interface{}{
								"storageClassName": "standard-csi",
							},
						},
					},
				},
			},
		})

		existingSC := &unstructured.Unstructured{}
		existingSC.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1",
			"kind":       "StorageClass",
			"metadata":   map[string]interface{}{"name": "standard-csi"},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.StorageClassGVR: "StorageClassList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, existingSC)

		issues := checkStorageClass(ctx, client, vm)
		s.Empty(issues)
	})

	s.Run("returns nil when no dataVolumeTemplates", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec":       map[string]interface{}{},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.StorageClassGVR: "StorageClassList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)

		issues := checkStorageClass(ctx, client, vm)
		s.Empty(issues)
	})
}

func (s *AnalysisSuite) TestCheckCloudInitCommands() {
	s.Run("detects shutdown command in cloud-init", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"runStrategy": "Always",
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - [\"shutdown\", \"-h\", \"now\"]\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Equal(severityCritical, issues[0].Severity)
		s.Contains(issues[0].Message, "shutdown")
		s.Contains(issues[0].Message, "crashloop")
		s.NotEmpty(issues[0].Fix)
	})

	s.Run("detects halt command", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - halt\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "halt")
	})

	s.Run("detects poweroff command", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - poweroff\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "poweroff")
	})

	s.Run("no issues for safe cloud-init", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\npackages:\n  - nginx\nruncmd:\n  - systemctl start nginx\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Empty(issues)
	})

	s.Run("detects systemctl poweroff command", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - systemctl poweroff\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "poweroff")
	})

	s.Run("detects systemctl reboot in array format without spaces", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - [systemctl,reboot]\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "reboot")
	})

	s.Run("detects unquoted YAML array format [shutdown, -h, now]", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"runStrategy": "Always",
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - [shutdown, -h, now]\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "shutdown")
	})

	s.Run("no false positive when dangerous word is argument not command", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - [logger, halt requested by user]\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Empty(issues)
	})

	s.Run("no false positive on comments or config keys", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\n# Do not shutdown the VM\nhostname: shutdown\npackages: [halt]\nbootcmd: []\nreboot_timeout: 30\nhalt_check: false\nruncmd:\n  - echo 'shutdown is not allowed'\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Empty(issues)
	})

	s.Run("shutdown -r is classified as reboot", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - shutdown -r now\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "reboot")
	})

	s.Run("accumulates all dangerous matches", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - shutdown -h now\n  - reboot\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Require().Len(issues, 2)
		s.Contains(issues[0].Message, "shutdown")
		s.Contains(issues[1].Message, "reboot")
	})

	s.Run("no false positive on non-dangerous shutdown flags", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cloudinitdisk",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config\nruncmd:\n  - shutdown -c\n",
								},
							},
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(vm, nil)
		s.Empty(issues)
	})

	s.Run("falls back to VMI when VM has no volumes", func() {
		vmi := &unstructured.Unstructured{}
		vmi.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstance",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"volumes": []interface{}{
					map[string]interface{}{
						"name": "cloudinitdisk",
						"cloudInitNoCloud": map[string]interface{}{
							"userData": "#cloud-config\nruncmd:\n  - shutdown -h now\n",
						},
					},
				},
			},
		})

		issues := checkCloudInitCommands(nil, vmi)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "shutdown")
	})

	s.Run("returns nil when no volumes", func() {
		issues := checkCloudInitCommands(nil, nil)
		s.Nil(issues)
	})
}

func (s *AnalysisSuite) TestCheckNodeSelector() {
	s.Run("detects hostname nodeSelector", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"kubernetes.io/hostname": "worker-0",
						},
					},
				},
			},
		})

		issues := checkNodeSelector(vm, nil)
		s.Require().Len(issues, 1)
		s.Equal(severityWarning, issues[0].Severity)
		s.Contains(issues[0].Message, "worker-0")
		s.Contains(issues[0].Message, "prevents live migration")
		s.NotEmpty(issues[0].Fix)
	})

	s.Run("reports non-hostname nodeSelector as info", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"node-role.kubernetes.io/worker": "",
						},
					},
				},
			},
		})

		issues := checkNodeSelector(vm, nil)
		s.Require().Len(issues, 1)
		s.Equal(severityInfo, issues[0].Severity)
	})

	s.Run("returns nil when no nodeSelector", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		})

		issues := checkNodeSelector(vm, nil)
		s.Empty(issues)
	})

	s.Run("falls back to VMI spec", func() {
		vmi := &unstructured.Unstructured{}
		vmi.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstance",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"spec": map[string]interface{}{
				"nodeSelector": map[string]interface{}{
					"kubernetes.io/hostname": "worker-1",
				},
			},
		})

		issues := checkNodeSelector(nil, vmi)
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "worker-1")
	})
}

func (s *AnalysisSuite) TestCheckPodHealth() {
	s.Run("detects Pending pod", func() {
		podDiags := []map[string]interface{}{
			{"phase": "Pending"},
		}

		issues := checkPodHealth(podDiags)
		s.Require().Len(issues, 1)
		s.Equal(severityWarning, issues[0].Severity)
		s.Contains(issues[0].Message, "Pending")
	})

	s.Run("detects CrashLoopBackOff container", func() {
		podDiags := []map[string]interface{}{
			{
				"phase": "Running",
				"containerStatuses": []map[string]interface{}{
					{
						"name":         "compute",
						"ready":        false,
						"restartCount": int64(5),
						"state": map[string]interface{}{
							"waiting": map[string]interface{}{
								"reason":  "CrashLoopBackOff",
								"message": "back-off 5m0s restarting failed container",
							},
						},
					},
				},
			},
		}

		issues := checkPodHealth(podDiags)
		s.Require().NotEmpty(issues)
		foundCrashloop := false
		for _, iss := range issues {
			if strings.Contains(iss.Message, "CrashLoopBackOff") {
				foundCrashloop = true
				s.Equal(severityCritical, iss.Severity)
			}
		}
		s.True(foundCrashloop)
	})

	s.Run("detects OOMKilled container", func() {
		podDiags := []map[string]interface{}{
			{
				"phase": "Running",
				"containerStatuses": []map[string]interface{}{
					{
						"name":         "compute",
						"ready":        false,
						"restartCount": int64(1),
						"state": map[string]interface{}{
							"terminated": map[string]interface{}{
								"reason": "OOMKilled",
							},
						},
					},
				},
			},
		}

		issues := checkPodHealth(podDiags)
		s.Require().NotEmpty(issues)
		found := false
		for _, iss := range issues {
			if strings.Contains(iss.Message, "OOMKilled") {
				found = true
				s.Equal(severityCritical, iss.Severity)
				s.NotEmpty(iss.Fix)
			}
		}
		s.True(found)
	})

	s.Run("detects crashloop with float64 restartCount (real cluster behavior)", func() {
		podDiags := []map[string]interface{}{
			{
				"phase": "Running",
				"containerStatuses": []map[string]interface{}{
					{
						"name":         "compute",
						"ready":        false,
						"restartCount": float64(7),
						"state": map[string]interface{}{
							"waiting": map[string]interface{}{
								"reason":  "CrashLoopBackOff",
								"message": "back-off restarting failed container",
							},
						},
					},
				},
			},
		}

		issues := checkPodHealth(podDiags)
		s.Require().NotEmpty(issues)
		foundRestart := false
		for _, iss := range issues {
			if strings.Contains(iss.Message, "restarted 7 times") {
				foundRestart = true
			}
		}
		s.True(foundRestart, "should detect restarts from float64 values (as returned by real Kubernetes API)")
	})

	s.Run("returns nil for healthy pod", func() {
		podDiags := []map[string]interface{}{
			{
				"phase": "Running",
				"containerStatuses": []map[string]interface{}{
					{
						"name":         "compute",
						"ready":        true,
						"restartCount": int64(0),
						"state": map[string]interface{}{
							"running": map[string]interface{}{},
						},
					},
				},
			},
		}

		issues := checkPodHealth(podDiags)
		s.Empty(issues)
	})

	s.Run("returns nil for nil diagnostics", func() {
		issues := checkPodHealth(nil)
		s.Nil(issues)
	})
}

func (s *AnalysisSuite) TestCheckMigrationStatus() {
	ctx := context.Background()

	s.Run("detects failed migration", func() {
		vmim := &unstructured.Unstructured{}
		vmim.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstanceMigration",
			"metadata": map[string]interface{}{
				"name":      "test-vm-migration",
				"namespace": "test-ns",
				"labels":    map[string]interface{}{"kubevirt.io/vmi-name": "test-vm"},
			},
			"spec": map[string]interface{}{
				"vmiName": "test-vm",
			},
			"status": map[string]interface{}{
				"phase": "Failed",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Failure",
						"status":  "True",
						"message": "cannot schedule: no suitable target node found",
					},
				},
			},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, vmim)

		issues := checkMigrationStatus(ctx, client, "test-ns", "test-vm")
		s.Require().Len(issues, 1)
		s.Equal(severityCritical, issues[0].Severity)
		s.Contains(issues[0].Message, "test-vm-migration")
		s.Contains(issues[0].Message, "failed")
		s.Contains(issues[0].Message, "no suitable target node found")
		s.NotEmpty(issues[0].Fix)
	})

	s.Run("ignores successful migrations", func() {
		vmim := &unstructured.Unstructured{}
		vmim.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstanceMigration",
			"metadata": map[string]interface{}{
				"name":      "test-vm-migration",
				"namespace": "test-ns",
				"labels":    map[string]interface{}{"kubevirt.io/vmi-name": "test-vm"},
			},
			"spec": map[string]interface{}{
				"vmiName": "test-vm",
			},
			"status": map[string]interface{}{
				"phase": "Succeeded",
			},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, vmim)

		issues := checkMigrationStatus(ctx, client, "test-ns", "test-vm")
		s.Empty(issues)
	})

	s.Run("ignores migrations for other VMs", func() {
		vmim := &unstructured.Unstructured{}
		vmim.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstanceMigration",
			"metadata": map[string]interface{}{
				"name":      "other-vm-migration",
				"namespace": "test-ns",
				"labels":    map[string]interface{}{"kubevirt.io/vmi-name": "other-vm"},
			},
			"spec": map[string]interface{}{
				"vmiName": "other-vm",
			},
			"status": map[string]interface{}{
				"phase": "Failed",
			},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, vmim)

		issues := checkMigrationStatus(ctx, client, "test-ns", "test-vm")
		s.Empty(issues)
	})

	s.Run("returns nil when no migrations exist", func() {
		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)

		issues := checkMigrationStatus(ctx, client, "test-ns", "test-vm")
		s.Empty(issues)
	})

	s.Run("includes age in failure message", func() {
		vmim := &unstructured.Unstructured{}
		vmim.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstanceMigration",
			"metadata": map[string]interface{}{
				"name":      "aged-migration",
				"namespace": "test-ns",
				"labels":    map[string]interface{}{"kubevirt.io/vmi-name": "test-vm"},
			},
			"spec": map[string]interface{}{
				"vmiName": "test-vm",
			},
			"status": map[string]interface{}{
				"phase": "Failed",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Failure",
						"status":  "True",
						"message": "scheduling issue",
					},
				},
			},
		})
		vmim.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-3 * time.Hour)))

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, vmim)

		issues := checkMigrationStatus(ctx, client, "test-ns", "test-vm")
		s.Require().Len(issues, 1)
		s.Contains(issues[0].Message, "aged-migration")
		s.Contains(issues[0].Message, "3h ago")
		s.Contains(issues[0].Message, "scheduling issue")
	})

	s.Run("reports only most recent 3 failures and notes skipped count", func() {
		var objs []runtime.Object
		for i := 0; i < 5; i++ {
			vmim := &unstructured.Unstructured{}
			vmim.SetUnstructuredContent(map[string]interface{}{
				"apiVersion": "kubevirt.io/v1",
				"kind":       "VirtualMachineInstanceMigration",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("mig-%d", i),
					"namespace": "test-ns",
					"labels":    map[string]interface{}{"kubevirt.io/vmi-name": "test-vm"},
				},
				"spec": map[string]interface{}{
					"vmiName": "test-vm",
				},
				"status": map[string]interface{}{
					"phase": "Failed",
				},
			})
			vmim.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-time.Duration(i) * time.Hour)))
			objs = append(objs, vmim)
		}

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, objs...)

		issues := checkMigrationStatus(ctx, client, "test-ns", "test-vm")
		s.Require().Len(issues, 4)
		s.Contains(issues[0].Message, "mig-0")
		s.Contains(issues[1].Message, "mig-1")
		s.Contains(issues[2].Message, "mig-2")
		s.Equal(severityInfo, issues[3].Severity)
		s.Contains(issues[3].Message, "2 additional older failed migrations not shown")
	})
}

func (s *AnalysisSuite) TestAnalyzeIssues() {
	ctx := context.Background()

	s.Run("returns no-issues message when all checks pass", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"printableStatus": "Running",
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.StorageClassGVR:                    "StorageClassList",
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)

		result := analyzeIssues(ctx, client, "test-ns", "test-vm", vm, nil, nil)
		s.Contains(result, "No issues automatically detected")
	})

	s.Run("produces Detected Issues and Suggested Fixes", func() {
		vm := &unstructured.Unstructured{}
		vm.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]interface{}{"name": "test-vm", "namespace": "test-ns"},
			"status": map[string]interface{}{
				"printableStatus": "Provisioning",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "VMINotExists",
						"message": "VMI does not exist",
					},
				},
				"volumeSnapshotStatuses": []interface{}{
					map[string]interface{}{
						"name":   "rootdisk",
						"reason": "PVC not found",
					},
				},
			},
			"spec": map[string]interface{}{
				"dataVolumeTemplates": []interface{}{
					map[string]interface{}{
						"metadata": map[string]interface{}{"name": "test-dv"},
						"spec": map[string]interface{}{
							"storage": map[string]interface{}{
								"storageClassName": "nonexistent-sc",
							},
						},
					},
				},
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		})

		gvrToListKind := map[schema.GroupVersionResource]string{
			kubevirt.StorageClassGVR:                    "StorageClassList",
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)

		result := analyzeIssues(ctx, client, "test-ns", "test-vm", vm, nil, nil)
		s.Contains(result, "## Detected Issues")
		s.Contains(result, "CRITICAL")
		s.Contains(result, "## Suggested Fixes")
	})
}
