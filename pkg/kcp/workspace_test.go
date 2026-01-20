package kcp

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type WorkspaceSuite struct {
	suite.Suite
}

func TestWorkspaceSuite(t *testing.T) {
	suite.Run(t, new(WorkspaceSuite))
}

func (s *WorkspaceSuite) TestParseServerURL() {
	s.Run("valid kcp server URL with root workspace", func() {
		baseURL, workspace := ParseServerURL("https://10.95.33.40:6443/clusters/root")
		s.Equal("https://10.95.33.40:6443", baseURL)
		s.Equal("root", workspace)
	})

	s.Run("valid kcp server URL with nested workspace", func() {
		baseURL, workspace := ParseServerURL("https://kcp.example.com:6443/clusters/root:org:team")
		s.Equal("https://kcp.example.com:6443", baseURL)
		s.Equal("root:org:team", workspace)
	})

	s.Run("valid kcp server URL with trailing slash", func() {
		baseURL, workspace := ParseServerURL("https://kcp.example.com:6443/clusters/root/")
		s.Equal("https://kcp.example.com:6443", baseURL)
		s.Equal("root", workspace)
	})

	s.Run("URL without clusters path returns empty workspace", func() {
		baseURL, workspace := ParseServerURL("https://kubernetes.example.com:6443")
		s.Equal("https://kubernetes.example.com:6443", baseURL)
		s.Equal("", workspace)
	})

	s.Run("empty URL", func() {
		baseURL, workspace := ParseServerURL("")
		s.Equal("", baseURL)
		s.Equal("", workspace)
	})
}

func (s *WorkspaceSuite) TestExtractWorkspaceFromURL() {
	s.Run("extracts workspace from valid URL", func() {
		workspace := ExtractWorkspaceFromURL("https://10.95.33.40:6443/clusters/root")
		s.Equal("root", workspace)
	})

	s.Run("extracts nested workspace from valid URL", func() {
		workspace := ExtractWorkspaceFromURL("https://kcp.example.com:6443/clusters/root:org:team")
		s.Equal("root:org:team", workspace)
	})

	s.Run("returns empty string for URL without clusters path", func() {
		workspace := ExtractWorkspaceFromURL("https://kubernetes.example.com:6443")
		s.Equal("", workspace)
	})

	s.Run("returns empty string for empty URL", func() {
		workspace := ExtractWorkspaceFromURL("")
		s.Equal("", workspace)
	})
}

func (s *WorkspaceSuite) TestConstructWorkspaceURL() {
	s.Run("constructs URL for root workspace", func() {
		url := ConstructWorkspaceURL("https://10.95.33.40:6443", "root")
		s.Equal("https://10.95.33.40:6443/clusters/root", url)
	})

	s.Run("constructs URL for nested workspace", func() {
		url := ConstructWorkspaceURL("https://kcp.example.com:6443", "root:org:team")
		s.Equal("https://kcp.example.com:6443/clusters/root:org:team", url)
	})

	s.Run("constructs URL with empty workspace", func() {
		url := ConstructWorkspaceURL("https://kcp.example.com:6443", "")
		s.Equal("https://kcp.example.com:6443/clusters/", url)
	})
}
