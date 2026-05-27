package features

import (
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/component-base/featuregate"
)

// Feature gate constants should be listed in alphabetical, case-sensitive
// (upper before any lower case character) order to reduce merge conflicts.
const (
// Add feature gate constants here, for example:
// MyFeature featuregate.Feature = "MyFeature"
)

func init() {
	if err := DefaultMutableFeatureGate.AddVersioned(defaultFeatureGates); err != nil {
		panic(err)
	}
}

// defaultFeatureGates contains the default feature gate definitions for this server.
// Each entry maps a Feature to its VersionedSpecs, which define the maturity
// lifecycle (Alpha -> Beta -> GA) across project versions.
//
// Entries should use version.MajorMinor(major, minor) for the Version field.
// Use version 0.0 for features that should be available regardless of emulation version.
//
// Example:
//
//	var defaultFeatureGates = map[featuregate.Feature]featuregate.VersionedSpecs{
//		MyFeature: {
//			{Version: version.MajorMinor(0, 1), Default: false, PreRelease: featuregate.Alpha},
//			{Version: version.MajorMinor(0, 5), Default: true, PreRelease: featuregate.Beta},
//			{Version: version.MajorMinor(1, 0), Default: true, PreRelease: featuregate.GA, LockToDefault: true},
//		},
//	}
var defaultFeatureGates = map[featuregate.Feature]featuregate.VersionedSpecs{
	// Add feature gate versioned specs here.
}

// Ensure version import is used (will be used when real features are added).
var _ = version.MajorMinor
