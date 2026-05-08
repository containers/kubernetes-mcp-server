package features

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/component-base/featuregate"

	pkgversion "github.com/containers/kubernetes-mcp-server/pkg/version"
)

var (
	// DefaultMutableFeatureGate is a mutable version of DefaultFeatureGate.
	// Only top-level command setup, tests, and the feature registration init()
	// should use this. Everything else should use DefaultFeatureGate.
	DefaultMutableFeatureGate featuregate.MutableVersionedFeatureGate = newDefaultFeatureGate()

	// DefaultFeatureGate is a shared global FeatureGate.
	// Callers should use this to check whether a given feature is enabled.
	DefaultFeatureGate featuregate.FeatureGate = DefaultMutableFeatureGate
)

func newDefaultFeatureGate() featuregate.MutableVersionedFeatureGate {
	ver, err := version.Parse(pkgversion.Version)
	if err != nil {
		// Fallback to 0.0 if the version string is not semver-parseable.
		ver = version.MajorMinor(0, 0)
	}
	// Use only major.minor for the emulation version.
	emulationVersion := version.MajorMinor(ver.Major(), ver.Minor())
	return featuregate.NewVersionedFeatureGate(emulationVersion)
}

// Enabled returns true if the given feature is enabled in the DefaultFeatureGate.
func Enabled(f featuregate.Feature) bool {
	return DefaultFeatureGate.Enabled(f)
}

// ApplyFeatureGates applies the given feature gate overrides to the DefaultMutableFeatureGate.
// This is the single entry point for applying feature gates from config or CLI.
// It is thread-safe: the underlying SetFromMap acquires an internal mutex and
// publishes changes via atomic.Store, so concurrent Enabled() calls are safe.
func ApplyFeatureGates(gates map[string]bool) error {
	if len(gates) == 0 {
		return nil
	}
	return DefaultMutableFeatureGate.SetFromMap(gates)
}

// ValidateFeatureGates validates that the given feature gate map contains only
// known features with valid values, without mutating the DefaultMutableFeatureGate.
// It performs a dry-run by applying gates to a deep copy of the gate.
func ValidateFeatureGates(gates map[string]bool) error {
	if len(gates) == 0 {
		return nil
	}
	copy := DefaultMutableFeatureGate.DeepCopy()
	if err := copy.SetFromMap(gates); err != nil {
		return fmt.Errorf("invalid feature gates: %w", err)
	}
	return nil
}
