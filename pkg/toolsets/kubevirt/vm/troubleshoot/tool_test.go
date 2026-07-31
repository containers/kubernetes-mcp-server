package troubleshoot

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type TroubleshootToolSuite struct {
	suite.Suite
}

func (s *TroubleshootToolSuite) TestRedactCloudInitSensitiveFields() {
	s.Run("redacts inline password value", func() {
		input := "#cloud-config\npassword: mysecret\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "password: <REDACTED>")
		s.NotContains(result, "mysecret")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts SSH key in list item without leaking material", func() {
		input := "ssh_authorized_keys:\n  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "ssh_authorized_keys: <REDACTED>")
		s.NotContains(result, "AAAAB3")
		s.NotContains(result, "user@host")
	})

	s.Run("redacts multi-line block scalar password", func() {
		input := "#cloud-config\npassword: |\n  multi-line-secret-value\n  second-line\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "password: <REDACTED>")
		s.NotContains(result, "multi-line-secret-value")
		s.NotContains(result, "second-line")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("preserves non-sensitive content", func() {
		input := "#cloud-config\nhostname: worker-1\nruncmd:\n  - dnf install -y httpd\n  - systemctl enable httpd\npackages:\n  - vim\n  - curl"
		result := redactCloudInitSensitiveFields(input)
		s.Equal(input, result)
	})

	s.Run("does not false-positive on comments containing sensitive words", func() {
		input := "#cloud-config\n# This is not a secret: just a comment\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "# This is not a secret: just a comment")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts ssh-ed25519 key in list", func() {
		input := "ssh_authorized_keys:\n  - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPrivateKey user@host"
		result := redactCloudInitSensitiveFields(input)
		s.NotContains(result, "AAAAPrivateKey")
		s.NotContains(result, "AAAAC3")
	})

	s.Run("redacts token field", func() {
		input := "#cloud-config\ntoken: abc123secret\nhostname: node1"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "token: <REDACTED>")
		s.NotContains(result, "abc123secret")
		s.Contains(result, "hostname: node1")
	})

	s.Run("redacts chpasswd block", func() {
		input := "#cloud-config\nchpasswd:\n  list:\n    - root:secret123\n    - user:pass456\n  expire: false\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "chpasswd: <REDACTED>")
		s.NotContains(result, "secret123")
		s.NotContains(result, "pass456")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts plain_text_passwd and hashed_passwd", func() {
		input := "#cloud-config\nplain_text_passwd: mypassword\nhashed_passwd: $6$rounds=4096$salt$hash\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "plain_text_passwd: <REDACTED>")
		s.Contains(result, "hashed_passwd: <REDACTED>")
		s.NotContains(result, "mypassword")
		s.NotContains(result, "$6$rounds")
	})

	s.Run("redacts ssh_keys host private keys", func() {
		input := "#cloud-config\nssh_keys:\n  rsa_private: |\n    -----BEGIN RSA PRIVATE KEY-----\n    MIIEpAIBAAKCAQEA...\n    -----END RSA PRIVATE KEY-----\n  rsa_public: ssh-rsa AAAAB3...\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "ssh_keys: <REDACTED>")
		s.NotContains(result, "MIIEpAIBAAKCAQEA")
		s.NotContains(result, "BEGIN RSA PRIVATE KEY")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts write_files block", func() {
		input := "#cloud-config\nwrite_files:\n  - path: /etc/ssl/private/key.pem\n    content: |\n      -----BEGIN PRIVATE KEY-----\n      MIIEvgIBADANBgkqhk...\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "write_files: <REDACTED>")
		s.NotContains(result, "MIIEvgIBADANBgkqhk")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts block scalar with indent indicator", func() {
		input := "#cloud-config\npassword: |2\n  indented-secret\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "password: <REDACTED>")
		s.NotContains(result, "indented-secret")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts block scalar with chomp and indent indicators", func() {
		input := "#cloud-config\npassword: >-\n  folded-secret\nhostname: myvm"
		result := redactCloudInitSensitiveFields(input)
		s.Contains(result, "password: <REDACTED>")
		s.NotContains(result, "folded-secret")
		s.Contains(result, "hostname: myvm")
	})

	s.Run("redacts ssh-dss key format", func() {
		input := "ssh_authorized_keys:\n  - ssh-dss AAAAB3NzaC1kc3MAAACBANcOo... user@host"
		result := redactCloudInitSensitiveFields(input)
		s.NotContains(result, "AAAAB3NzaC1kc3M")
	})

	s.Run("redacts PEM certificate blocks", func() {
		input := "#cloud-config\nruncmd:\n  - echo '-----BEGIN CERTIFICATE-----' > /tmp/cert"
		result := redactCloudInitSensitiveFields(input)
		s.NotContains(result, "CERTIFICATE")
	})
}

func (s *TroubleshootToolSuite) TestStripCloudInitBodies() {
	s.Run("replaces userData and networkData with placeholder", func() {
		volumes := []any{
			map[string]interface{}{
				"name": "cloudinitdisk",
				"cloudInitNoCloud": map[string]interface{}{
					"userData":    "#cloud-config\npassword: secret123",
					"networkData": "network:\n  version: 2",
				},
			},
			map[string]interface{}{
				"name": "rootdisk",
				"containerDisk": map[string]interface{}{
					"image": "registry.io/image:latest",
				},
			},
		}

		result := stripCloudInitBodies(volumes)
		s.Require().Len(result, 2)

		ciVol := result[0].(map[string]interface{})
		ciData := ciVol["cloudInitNoCloud"].(map[string]interface{})
		s.Equal("<see Cloud-Init Configuration section>", ciData["userData"])
		s.Equal("<see Cloud-Init Configuration section>", ciData["networkData"])

		rootVol := result[1].(map[string]interface{})
		cdData := rootVol["containerDisk"].(map[string]interface{})
		s.Equal("registry.io/image:latest", cdData["image"])
	})

	s.Run("preserves secretRef in cloud-init", func() {
		volumes := []any{
			map[string]interface{}{
				"name": "cloudinitdisk",
				"cloudInitNoCloud": map[string]interface{}{
					"userDataSecretRef": map[string]interface{}{"name": "my-secret"},
				},
			},
		}

		result := stripCloudInitBodies(volumes)
		ciVol := result[0].(map[string]interface{})
		ciData := ciVol["cloudInitNoCloud"].(map[string]interface{})
		ref := ciData["userDataSecretRef"].(map[string]interface{})
		s.Equal("my-secret", ref["name"])
	})

	s.Run("replaces base64 userData and networkData variants", func() {
		volumes := []any{
			map[string]interface{}{
				"name": "cloudinitdisk",
				"cloudInitNoCloud": map[string]interface{}{
					"userDataBase64":    base64.StdEncoding.EncodeToString([]byte("#cloud-config\npassword: secret123")),
					"networkDataBase64": base64.StdEncoding.EncodeToString([]byte("network:\n  version: 2")),
				},
			},
		}

		result := stripCloudInitBodies(volumes)
		ciVol := result[0].(map[string]interface{})
		ciData := ciVol["cloudInitNoCloud"].(map[string]interface{})
		s.Equal("<see Cloud-Init Configuration section>", ciData["userDataBase64"])
		s.Equal("<see Cloud-Init Configuration section>", ciData["networkDataBase64"])
	})
}

func (s *TroubleshootToolSuite) TestCloudInitData() {
	s.Run("prefers plain-text over base64", func() {
		ciData := map[string]interface{}{
			"userData":       "#cloud-config\nplain",
			"userDataBase64": base64.StdEncoding.EncodeToString([]byte("#cloud-config\nencoded")),
		}
		s.Equal("#cloud-config\nplain", cloudInitData(ciData, "userData", "userDataBase64"))
	})

	s.Run("decodes base64 variant when plain-text absent", func() {
		ciData := map[string]interface{}{
			"userDataBase64": base64.StdEncoding.EncodeToString([]byte("#cloud-config\npassword: secret123")),
		}
		s.Equal("#cloud-config\npassword: secret123", cloudInitData(ciData, "userData", "userDataBase64"))
	})

	s.Run("returns empty for invalid base64", func() {
		ciData := map[string]interface{}{"userDataBase64": "not valid base64 !!!"}
		s.Equal("", cloudInitData(ciData, "userData", "userDataBase64"))
	})

	s.Run("returns empty when neither present", func() {
		s.Equal("", cloudInitData(map[string]interface{}{}, "userData", "userDataBase64"))
	})
}

func (s *TroubleshootToolSuite) TestRedactLogSecrets() {
	s.Run("redacts bearer token in logs", func() {
		input := "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.abc"
		result := redactLogSecrets(input)
		s.Contains(result, "Bearer <REDACTED>")
		s.NotContains(result, "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9")
	})

	s.Run("preserves normal log lines", func() {
		input := "2026-07-20 10:00:00 INFO VM started successfully"
		result := redactLogSecrets(input)
		s.Equal(input, result)
	})

	s.Run("redacts password= in logs", func() {
		input := "connecting with password=mysecret123"
		result := redactLogSecrets(input)
		s.Contains(result, "password=<REDACTED>")
		s.NotContains(result, "mysecret123")
	})
}

func (s *TroubleshootToolSuite) TestIsBlockScalarIndicator() {
	s.Run("recognizes standard indicators", func() {
		s.True(isBlockScalarIndicator("|"))
		s.True(isBlockScalarIndicator(">"))
		s.True(isBlockScalarIndicator("|+"))
		s.True(isBlockScalarIndicator("|-"))
		s.True(isBlockScalarIndicator(">+"))
		s.True(isBlockScalarIndicator(">-"))
		s.True(isBlockScalarIndicator("|2"))
		s.True(isBlockScalarIndicator(">2"))
		s.True(isBlockScalarIndicator("|2-"))
		s.True(isBlockScalarIndicator("|-2"))
	})

	s.Run("rejects non-indicators", func() {
		s.False(isBlockScalarIndicator(""))
		s.False(isBlockScalarIndicator("hello"))
		s.False(isBlockScalarIndicator("true"))
		s.False(isBlockScalarIndicator("123"))
	})
}

func (s *TroubleshootToolSuite) TestExtractPodDiagnostics() {
	s.Run("extracts only diagnostic fields from pod", func() {
		pod := &unstructured.Unstructured{}
		pod.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "virt-launcher-test-vm-abc123",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"nodeName": "worker-1",
				"nodeSelector": map[string]interface{}{
					"kubernetes.io/hostname": "worker-1",
				},
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "compute",
						"image": "registry.io/kubevirt/virt-launcher:v1.0.0",
						"env": []interface{}{
							map[string]interface{}{"name": "SECRET_VAR", "value": "should-not-appear"},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"phase": "Running",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":         "compute",
						"ready":        true,
						"restartCount": int64(3),
						"state": map[string]interface{}{
							"running": map[string]interface{}{
								"startedAt": "2026-06-29T10:00:00Z",
							},
						},
						"lastTerminationState": map[string]interface{}{
							"terminated": map[string]interface{}{
								"exitCode": int64(137),
								"reason":   "OOMKilled",
							},
						},
					},
				},
			},
		})

		diag := extractPodDiagnostics(pod)

		s.Equal("Running", diag["phase"])
		s.Equal("worker-1", diag["nodeName"])
		s.NotNil(diag["conditions"])
		s.NotNil(diag["containerStatuses"])
		s.NotNil(diag["nodeSelector"])

		_, hasContainers := diag["containers"]
		s.False(hasContainers, "should not include full container specs")

		statuses := diag["containerStatuses"].([]map[string]interface{})
		s.Require().Len(statuses, 1)
		s.Equal(int64(3), statuses[0]["restartCount"])
		s.Equal("compute", statuses[0]["name"])
	})

	s.Run("excludes env vars and image from output", func() {
		pod := &unstructured.Unstructured{}
		pod.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "virt-launcher-test-vm-xyz",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"nodeName": "worker-2",
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "compute",
						"image": "registry.io/kubevirt/virt-launcher:v1.2.3",
						"env": []interface{}{
							map[string]interface{}{"name": "API_KEY", "value": "secret-value-123"},
						},
						"volumeMounts": []interface{}{
							map[string]interface{}{"name": "disk0", "mountPath": "/var/run/kubevirt"},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"phase": "Running",
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":         "compute",
						"ready":        true,
						"restartCount": int64(0),
						"state": map[string]interface{}{
							"running": map[string]interface{}{},
						},
					},
				},
			},
		})

		diag := extractPodDiagnostics(pod)

		s.Equal("Running", diag["phase"])
		s.Equal("worker-2", diag["nodeName"])

		_, hasContainers := diag["containers"]
		s.False(hasContainers, "should not include container specs")

		_, hasSpec := diag["spec"]
		s.False(hasSpec, "should not include raw spec")

		statuses := diag["containerStatuses"].([]map[string]interface{})
		s.Require().Len(statuses, 1)
		_, hasImage := statuses[0]["image"]
		s.False(hasImage, "container status summary should not include image")
	})
}

func TestTroubleshootToolSuite(t *testing.T) {
	suite.Run(t, new(TroubleshootToolSuite))
}
