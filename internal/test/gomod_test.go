package test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/suite"
	"golang.org/x/mod/modfile"
)

type GoModSuite struct {
	suite.Suite
	parsed *modfile.File
}

func (s *GoModSuite) SetupSuite() {
	_, thisFile, _, ok := runtime.Caller(0)
	s.Require().True(ok, "Expected to determine test file location")
	goModPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "go.mod")
	data, err := os.ReadFile(goModPath)
	s.Require().NoError(err, "Expected to read root go.mod")
	// Strict Parse is required here: ParseLax silently drops main-module-only
	// directives (replace and exclude) before recording them, which are exactly
	// the directives this guard inspects, so it would make the assertions below
	// permanent no-ops. Strict Parse surfaces them. Its only downside is that it
	// errors on a directive unknown to the pinned x/mod version, i.e. a brand-new
	// Go directive — a loud, one-line x/mod bump to fix, far preferable to a
	// guard that can never fail.
	s.parsed, err = modfile.Parse(goModPath, data, nil)
	s.Require().NoError(err, "Expected to parse root go.mod")
}

// TestInstallableAsDependency guards against re-introducing directives that make
// the module impossible to `go install <module>/...@version` (or `go get` as a
// dependency). Go refuses such an install when the target module's go.mod
// "contains one or more replace directives" — and the same restriction applies
// to exclude directives, since both would cause the module to be interpreted
// differently than if it were the main module (see `go help install`). Either
// directive therefore breaks installing the server from source for downstream
// consumers, even though neither has any effect on `make build` of a local
// clone. See issue #1231.
func (s *GoModSuite) TestInstallableAsDependency() {
	s.Run("go.mod has no replace directives", func() {
		replaced := make([]string, 0, len(s.parsed.Replace))
		for _, r := range s.parsed.Replace {
			replaced = append(replaced, r.Old.Path)
		}
		s.Emptyf(replaced, "go.mod must not contain replace directives (found %v); they break `go install <module>/...@version` for downstream consumers (see issue #1231)", replaced)
	})
	s.Run("go.mod has no exclude directives", func() {
		excluded := make([]string, 0, len(s.parsed.Exclude))
		for _, e := range s.parsed.Exclude {
			excluded = append(excluded, e.Mod.Path)
		}
		s.Emptyf(excluded, "go.mod must not contain exclude directives (found %v); they break `go install <module>/...@version` for downstream consumers (see issue #1231)", excluded)
	})
}

func TestGoMod(t *testing.T) {
	suite.Run(t, new(GoModSuite))
}
