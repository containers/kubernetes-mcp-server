package create

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type CreateToolSuite struct {
	suite.Suite
}

func (s *CreateToolSuite) TestParseNetworks() {
	s.Run("nil input returns nil", func() {
		networks, err := parseNetworks(nil)
		s.NoError(err)
		s.Nil(networks)
	})

	s.Run("empty array returns nil", func() {
		networks, err := parseNetworks([]any{})
		s.NoError(err)
		s.Nil(networks)
	})

	s.Run("simple network name as string", func() {
		networks, err := parseNetworks([]any{"vlan-network"})
		s.NoError(err)
		s.Require().Len(networks, 1)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
	})

	s.Run("multiple network names as strings", func() {
		networks, err := parseNetworks([]any{"vlan-network", "storage-network"})
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
		s.Equal("storage-network", networks[1].Name)
		s.Equal("storage-network", networks[1].NetworkName)
	})

	s.Run("empty strings are skipped", func() {
		networks, err := parseNetworks([]any{"vlan-network", "", "storage-network"})
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("storage-network", networks[1].Name)
	})

	s.Run("object with name and networkName", func() {
		networks, err := parseNetworks([]any{
			map[string]any{"name": "vlan100", "networkName": "vlan-network"},
		})
		s.NoError(err)
		s.Require().Len(networks, 1)
		s.Equal("vlan100", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
	})

	s.Run("multiple objects", func() {
		networks, err := parseNetworks([]any{
			map[string]any{"name": "vlan100", "networkName": "vlan-network"},
			map[string]any{"name": "storage", "networkName": "storage-network"},
		})
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan100", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
		s.Equal("storage", networks[1].Name)
		s.Equal("storage-network", networks[1].NetworkName)
	})

	s.Run("object with only networkName uses it as name", func() {
		networks, err := parseNetworks([]any{
			map[string]any{"networkName": "vlan-network"},
		})
		s.NoError(err)
		s.Require().Len(networks, 1)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
	})

	s.Run("object missing networkName returns error", func() {
		_, err := parseNetworks([]any{
			map[string]any{"name": "vlan100"},
		})
		s.Error(err)
		s.Contains(err.Error(), "missing required 'networkName' field")
	})

	s.Run("mixed strings and objects", func() {
		networks, err := parseNetworks([]any{
			"simple-network",
			map[string]any{"name": "vlan100", "networkName": "vlan-network"},
		})
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("simple-network", networks[0].Name)
		s.Equal("simple-network", networks[0].NetworkName)
		s.Equal("vlan100", networks[1].Name)
		s.Equal("vlan-network", networks[1].NetworkName)
	})

	s.Run("invalid type returns error", func() {
		_, err := parseNetworks([]any{123})
		s.Error(err)
		s.Contains(err.Error(), "invalid type")
	})
}

func (s *CreateToolSuite) TestRenderVMYaml() {
	s.Run("VM without networks", func() {
		params := vmParams{
			Namespace:     "test-ns",
			Name:          "test-vm",
			ContainerDisk: "quay.io/containerdisks/fedora:latest",
			RunStrategy:   "Halted",
		}
		yaml, err := renderVMYaml(params)
		s.NoError(err)
		s.Contains(yaml, "name: test-vm")
		s.Contains(yaml, "namespace: test-ns")
		s.Contains(yaml, "runStrategy: Halted")
		s.Contains(yaml, "image: quay.io/containerdisks/fedora:latest")
		s.NotContains(yaml, "networks:")
		s.NotContains(yaml, "interfaces:")
	})

	s.Run("VM with single network", func() {
		params := vmParams{
			Namespace:     "test-ns",
			Name:          "test-vm",
			ContainerDisk: "quay.io/containerdisks/fedora:latest",
			RunStrategy:   "Halted",
			Networks: []NetworkConfig{
				{Name: "vlan-network", NetworkName: "vlan-network"},
			},
		}
		yaml, err := renderVMYaml(params)
		s.NoError(err)
		s.Contains(yaml, "networks:")
		s.Contains(yaml, "- name: vlan-network")
		s.Contains(yaml, "networkName: vlan-network")
		s.Contains(yaml, "interfaces:")
		s.Contains(yaml, "bridge: {}")
	})

	s.Run("VM with multiple networks", func() {
		params := vmParams{
			Namespace:     "test-ns",
			Name:          "test-vm",
			ContainerDisk: "quay.io/containerdisks/fedora:latest",
			RunStrategy:   "Halted",
			Networks: []NetworkConfig{
				{Name: "vlan100", NetworkName: "vlan-network"},
				{Name: "storage", NetworkName: "storage-network"},
			},
		}
		yaml, err := renderVMYaml(params)
		s.NoError(err)
		s.Contains(yaml, "- name: vlan100")
		s.Contains(yaml, "networkName: vlan-network")
		s.Contains(yaml, "- name: storage")
		s.Contains(yaml, "networkName: storage-network")
	})
}

func TestCreateToolSuite(t *testing.T) {
	suite.Run(t, new(CreateToolSuite))
}
