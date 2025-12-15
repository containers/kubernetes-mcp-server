package api

import (
	"go/build"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

const modulePrefix = "github.com/containers/kubernetes-mcp-server/"

// ImportsSuite verifies that pkg/api doesn't accidentally import internal packages
// that would create cyclic dependencies.
type ImportsSuite struct {
	suite.Suite
}

func (s *ImportsSuite) TestNoCyclicDependencies() {
	// Whitelist of allowed internal packages that pkg/api can import.
	// Any other internal import will cause the test to fail.
	allowedInternalPackages := map[string]bool{
		"github.com/containers/kubernetes-mcp-server/pkg/output": true,
	}

	s.Run("pkg/api only imports whitelisted internal packages", func() {
		pkg, err := build.Import("github.com/containers/kubernetes-mcp-server/pkg/api", "", 0)
		s.Require().NoError(err, "Failed to import pkg/api")

		for _, imp := range pkg.Imports {
			// Skip external packages (not part of this module)
			if !strings.HasPrefix(imp, modulePrefix) {
				continue
			}

			// Internal package - must be in whitelist
			if !allowedInternalPackages[imp] {
				s.Failf("Forbidden internal import detected",
					"pkg/api imports %q which is not in the whitelist. "+
						"To prevent cyclic dependencies, pkg/api can only import: %v. "+
						"If this import is intentional, add it to allowedInternalPackages in this test.",
					imp, keys(allowedInternalPackages))
			}
		}
	})
}

func keys(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func TestImports(t *testing.T) {
	suite.Run(t, new(ImportsSuite))
}
