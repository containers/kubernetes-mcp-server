package create

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type CreateToolSuite struct {
	suite.Suite
}

func (s *CreateToolSuite) TestParseNetworks() {
	s.Run("empty input returns nil", func() {
		networks, err := parseNetworks("")
		s.NoError(err)
		s.Nil(networks)
	})

	s.Run("whitespace only returns nil", func() {
		networks, err := parseNetworks("   ")
		s.NoError(err)
		s.Nil(networks)
	})

	s.Run("simple network name", func() {
		networks, err := parseNetworks("vlan-network")
		s.NoError(err)
		s.Require().Len(networks, 1)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
	})

	s.Run("comma-separated network names", func() {
		networks, err := parseNetworks("vlan-network,storage-network")
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
		s.Equal("storage-network", networks[1].Name)
		s.Equal("storage-network", networks[1].NetworkName)
	})

	s.Run("comma-separated with whitespace", func() {
		networks, err := parseNetworks("vlan-network , storage-network")
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("storage-network", networks[1].Name)
	})

	s.Run("comma-separated with empty entries", func() {
		networks, err := parseNetworks("vlan-network,,storage-network,")
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("storage-network", networks[1].Name)
	})

	s.Run("JSON array format", func() {
		networks, err := parseNetworks(`[{"name":"vlan100","networkName":"vlan-network"}]`)
		s.NoError(err)
		s.Require().Len(networks, 1)
		s.Equal("vlan100", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
	})

	s.Run("JSON array with multiple networks", func() {
		networks, err := parseNetworks(`[{"name":"vlan100","networkName":"vlan-network"},{"name":"storage","networkName":"storage-network"}]`)
		s.NoError(err)
		s.Require().Len(networks, 2)
		s.Equal("vlan100", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
		s.Equal("storage", networks[1].Name)
		s.Equal("storage-network", networks[1].NetworkName)
	})

	s.Run("JSON array with only networkName uses it as name", func() {
		networks, err := parseNetworks(`[{"networkName":"vlan-network"}]`)
		s.NoError(err)
		s.Require().Len(networks, 1)
		s.Equal("vlan-network", networks[0].Name)
		s.Equal("vlan-network", networks[0].NetworkName)
	})

	s.Run("JSON array missing networkName returns error", func() {
		_, err := parseNetworks(`[{"name":"vlan100"}]`)
		s.Error(err)
		s.Contains(err.Error(), "missing required 'networkName' field")
	})

	s.Run("invalid JSON returns error", func() {
		_, err := parseNetworks(`[{"name":"vlan100"`)
		s.Error(err)
		s.Contains(err.Error(), "failed to parse networks JSON")
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
