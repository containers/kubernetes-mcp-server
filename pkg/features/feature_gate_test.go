package features

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/stretchr/testify/suite"
)

const (
	testFeatureAlpha featuregate.Feature = "TestFeatureAlpha"
	testFeatureBeta  featuregate.Feature = "TestFeatureBeta"
)

type FeatureGateSuite struct {
	suite.Suite
}

func (s *FeatureGateSuite) SetupSuite() {
	// Register test features once. AddVersioned is idempotent for identical specs.
	err := DefaultMutableFeatureGate.AddVersioned(map[featuregate.Feature]featuregate.VersionedSpecs{
		testFeatureAlpha: {
			{Version: version.MajorMinor(0, 0), Default: false, PreRelease: featuregate.Alpha},
		},
		testFeatureBeta: {
			{Version: version.MajorMinor(0, 0), Default: true, PreRelease: featuregate.Beta},
		},
	})
	s.Require().NoError(err)
}

func (s *FeatureGateSuite) TestDefaultFeatureGateIsNotNull() {
	s.NotNil(DefaultFeatureGate, "DefaultFeatureGate should not be nil")
	s.NotNil(DefaultMutableFeatureGate, "DefaultMutableFeatureGate should not be nil")
}

func (s *FeatureGateSuite) TestAlphaFeatureDefaultDisabled() {
	s.Run("alpha feature is disabled by default", func() {
		featuregatetesting.SetFeatureGateDuringTest(s.T(), DefaultMutableFeatureGate, testFeatureAlpha, false)
		s.False(Enabled(testFeatureAlpha))
	})
}

func (s *FeatureGateSuite) TestBetaFeatureDefaultEnabled() {
	s.Run("beta feature is enabled by default", func() {
		featuregatetesting.SetFeatureGateDuringTest(s.T(), DefaultMutableFeatureGate, testFeatureBeta, true)
		s.True(Enabled(testFeatureBeta))
	})
}

func (s *FeatureGateSuite) TestSetFeatureGateDuringTest() {
	s.Run("can enable alpha feature during test", func() {
		featuregatetesting.SetFeatureGateDuringTest(s.T(), DefaultMutableFeatureGate, testFeatureAlpha, true)
		s.True(Enabled(testFeatureAlpha))
	})
	s.Run("can disable beta feature during test", func() {
		featuregatetesting.SetFeatureGateDuringTest(s.T(), DefaultMutableFeatureGate, testFeatureBeta, false)
		s.False(Enabled(testFeatureBeta))
	})
}

func (s *FeatureGateSuite) TestApplyFeatureGates() {
	s.Run("applies feature gate overrides", func() {
		featuregatetesting.SetFeatureGateDuringTest(s.T(), DefaultMutableFeatureGate, testFeatureAlpha, false)
		err := ApplyFeatureGates(map[string]bool{
			string(testFeatureAlpha): true,
		})
		s.Require().NoError(err)
		s.True(Enabled(testFeatureAlpha))
	})
	s.Run("returns nil for empty map", func() {
		s.Nil(ApplyFeatureGates(map[string]bool{}))
	})
	s.Run("returns nil for nil map", func() {
		s.Nil(ApplyFeatureGates(nil))
	})
}

func (s *FeatureGateSuite) TestValidateFeatureGates() {
	s.Run("validates known features", func() {
		err := ValidateFeatureGates(map[string]bool{
			string(testFeatureAlpha): true,
		})
		s.NoError(err)
	})
	s.Run("returns error for unknown feature", func() {
		err := ValidateFeatureGates(map[string]bool{
			"CompletelyUnknown": true,
		})
		s.Error(err)
		s.Contains(err.Error(), "CompletelyUnknown")
	})
	s.Run("does not mutate the default gate", func() {
		featuregatetesting.SetFeatureGateDuringTest(s.T(), DefaultMutableFeatureGate, testFeatureAlpha, false)
		_ = ValidateFeatureGates(map[string]bool{
			string(testFeatureAlpha): true,
		})
		s.False(Enabled(testFeatureAlpha), "ValidateFeatureGates should not mutate the default gate")
	})
	s.Run("returns nil for empty map", func() {
		s.Nil(ValidateFeatureGates(map[string]bool{}))
	})
	s.Run("returns nil for nil map", func() {
		s.Nil(ValidateFeatureGates(nil))
	})
}

func (s *FeatureGateSuite) TestAllAlphaToggle() {
	s.Run("AllAlpha enables all alpha features", func() {
		featuregatetesting.SetFeatureGatesDuringTest(s.T(), DefaultMutableFeatureGate, map[featuregate.Feature]bool{
			"AllAlpha": true,
		})
		s.True(Enabled(testFeatureAlpha))
	})
	s.Run("AllAlpha does not affect beta features", func() {
		featuregatetesting.SetFeatureGatesDuringTest(s.T(), DefaultMutableFeatureGate, map[featuregate.Feature]bool{
			"AllAlpha": true,
		})
		s.True(Enabled(testFeatureBeta), "beta feature should remain enabled")
	})
}

func (s *FeatureGateSuite) TestKnownFeatures() {
	known := DefaultFeatureGate.KnownFeatures()
	s.NotEmpty(known, "KnownFeatures should include at least AllAlpha/AllBeta")
}

// TestUnknownFeatureGateError tests error handling for unknown features using
// a separate feature gate instance to avoid poisoning the shared DefaultMutableFeatureGate.
// The upstream SetFromMap stores raw entries into enabledRaw before validation,
// so an unknown feature would corrupt the global gate for subsequent tests.
func (s *FeatureGateSuite) TestUnknownFeatureGateError() {
	s.Run("ApplyFeatureGates with unknown feature returns error via validate", func() {
		err := ValidateFeatureGates(map[string]bool{
			"UnknownFeature": true,
		})
		s.Error(err)
		s.Contains(err.Error(), "UnknownFeature")
	})
}

func TestFeatureGate(t *testing.T) {
	suite.Run(t, new(FeatureGateSuite))
}
