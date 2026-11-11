package kubernetes

import (
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// restMapperResetInterval bounds how often a NoMatch resets the REST mapper.
const restMapperResetInterval = 30 * time.Second

// refreshingRESTMapper wraps a discovery-backed REST mapper and, when enabled, resets
// it and retries once on a NoMatch, so resources added to the cluster after the mapper
// was loaded are found without a restart. Resets are bounded by restMapperResetInterval.
type refreshingRESTMapper struct {
	delegate       meta.ResettableRESTMapper
	resetOnNoMatch bool
	lastReset      atomic.Int64
}

var _ meta.ResettableRESTMapper = (*refreshingRESTMapper)(nil)

// newRefreshingRESTMapper wraps delegate. Set resetOnNoMatch for long-lived clients
// whose mapper can go stale; per-request clients build a fresh mapper and leave it unset.
func newRefreshingRESTMapper(delegate meta.ResettableRESTMapper, resetOnNoMatch bool) *refreshingRESTMapper {
	return &refreshingRESTMapper{delegate: delegate, resetOnNoMatch: resetOnNoMatch}
}

func (m *refreshingRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return retryOnNoMatch(m, func() (schema.GroupVersionKind, error) { return m.delegate.KindFor(resource) })
}

func (m *refreshingRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return retryOnNoMatch(m, func() ([]schema.GroupVersionKind, error) { return m.delegate.KindsFor(resource) })
}

func (m *refreshingRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return retryOnNoMatch(m, func() (schema.GroupVersionResource, error) { return m.delegate.ResourceFor(input) })
}

func (m *refreshingRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return retryOnNoMatch(m, func() ([]schema.GroupVersionResource, error) { return m.delegate.ResourcesFor(input) })
}

func (m *refreshingRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return retryOnNoMatch(m, func() (*meta.RESTMapping, error) { return m.delegate.RESTMapping(gk, versions...) })
}

func (m *refreshingRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return retryOnNoMatch(m, func() ([]*meta.RESTMapping, error) { return m.delegate.RESTMappings(gk, versions...) })
}

func (m *refreshingRESTMapper) ResourceSingularizer(resource string) (string, error) {
	return m.delegate.ResourceSingularizer(resource)
}

// Reset resets the delegate so the next lookup reloads discovery.
func (m *refreshingRESTMapper) Reset() {
	m.delegate.Reset()
}

func retryOnNoMatch[T any](m *refreshingRESTMapper, lookup func() (T, error)) (T, error) {
	result, err := lookup()
	if meta.IsNoMatchError(err) && m.resetForNoMatch() {
		return lookup()
	}
	return result, err
}

// resetForNoMatch resets the delegate at most once per restMapperResetInterval and
// reports whether it did.
func (m *refreshingRESTMapper) resetForNoMatch() bool {
	if !m.resetOnNoMatch {
		return false
	}
	now := time.Now().UnixNano()
	last := m.lastReset.Load()
	if last != 0 && now-last < int64(restMapperResetInterval) {
		return false
	}
	if !m.lastReset.CompareAndSwap(last, now) {
		return false
	}
	m.delegate.Reset()
	return true
}
