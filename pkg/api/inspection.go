package api

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ClusterInspector interface {
	Discovery() AggregateDiscovery
	Unstructured() AggregateUnstructured
}

type AggregateDiscovery interface {
	ServerResourcesForGroupVersion(
		ctx context.Context,
		groupVersion string,
	) Results[*metav1.APIResourceList]
}

// AggregateGroupDiscovery is an optional interface for discovering resources
// across all served versions of an API group.
type AggregateGroupDiscovery interface {
	ServerResourcesForGroup(ctx context.Context, group string) Results[[]*metav1.APIResourceList]
}

// AnyTargetHasGVK reports whether any target exposes the given GVK.
// An empty Version matches any served version. Inspectors without group discovery
// support retain tools conservatively when the version is unspecified.
//
// AnyTargetHasGVK fails open on errors, so that we do not hide tools that _may_ be valid
// This is primarily useful for deciding to disable tools that depend on GVKs when those GVKs
// are definitely not present.
func AnyTargetHasGVK(ctx context.Context, inspector ClusterInspector, gvk schema.GroupVersionKind) bool {
	if gvk.Version == "" {
		groupDiscovery, ok := inspector.Discovery().(AggregateGroupDiscovery)
		if !ok {
			return true
		}
		hasGVK, err := groupDiscovery.ServerResourcesForGroup(ctx, gvk.Group).Any(func(lists []*metav1.APIResourceList) bool {
			for _, list := range lists {
				for _, resource := range list.APIResources {
					if resource.Kind == gvk.Kind {
						return true
					}
				}
			}
			return false
		})
		return hasGVK || (err != nil && !IsNotFound(err))
	}
	hasGVK, err := inspector.Discovery().ServerResourcesForGroupVersion(ctx, gvk.GroupVersion().String()).Any(func(list *metav1.APIResourceList) bool {
		for _, resource := range list.APIResources {
			if resource.Kind == gvk.Kind {
				return true
			}
		}
		return false
	})

	return hasGVK || (err != nil && !IsNotFound(err))
}

type AggregateUnstructured interface {
	Resource(resource schema.GroupVersionResource) AggregateNamespaceableResourceInterface
}

type AggregateNamespaceableResourceInterface interface {
	Namespace(string) AggregateResourceInterface
	AggregateResourceInterface
}

type AggregateResourceInterface interface {
	Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) Results[*unstructured.Unstructured]
	List(ctx context.Context, opts metav1.ListOptions) Results[*unstructured.UnstructuredList]
}

type Results[T any] struct {
	ctx            context.Context
	targetProvider TargetProvider
	run            func(ctx context.Context, target string) (T, error)
}

func NewResults[T any](
	ctx context.Context,
	targetProvider TargetProvider,
	run func(context.Context, string) (T, error),
) Results[T] {
	return Results[T]{
		ctx:            ctx,
		targetProvider: targetProvider,
		run:            run,
	}
}

// Any returns true if any of the results match
//
// Any only aggregates and returns errors if none of the results match the predicate
func (r Results[T]) Any(match func(T) bool) (bool, error) {
	targets, err := r.targetProvider.GetTargets(r.ctx)
	if err != nil {
		return false, fmt.Errorf("failed to fetch targets: %w", err)
	}

	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	results := make(chan result[T], len(targets))

	for _, target := range targets {
		go func() {
			value, err := r.run(ctx, target)
			results <- result[T]{value: value, err: err}
		}()
	}

	for range targets {
		result := <-results
		if result.err != nil {
			err = errors.Join(err, result.err)
			continue
		}
		if match(result.value) {
			return true, nil
		}
	}

	return false, err
}

// All returns true if all of the results match.
//
// All only aggregates and returns errors if all of the results match the predicate
func (r Results[T]) All(match func(T) bool) (bool, error) {
	targets, err := r.targetProvider.GetTargets(r.ctx)
	if err != nil {
		return false, fmt.Errorf("failed to fetch targets: %w", err)
	}

	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	results := make(chan result[T], len(targets))

	for _, target := range targets {
		go func() {
			value, err := r.run(ctx, target)
			results <- result[T]{value: value, err: err}
		}()
	}

	for range targets {
		result := <-results
		if result.err != nil {
			err = errors.Join(err, result.err)
			continue
		}

		if !match(result.value) {
			return false, nil
		}
	}

	return err == nil, err
}

// Default evaluates the result on the default target.
func (r Results[T]) Default() (T, error) {
	return r.run(r.ctx, r.targetProvider.GetDefaultTarget())
}

// Values evaluates the results on all targets and aggregates errors
func (r Results[T]) Values() ([]T, error) {
	targets, err := r.targetProvider.GetTargets(r.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch targets: %w", err)
	}

	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	results := make(chan result[T], len(targets))

	for _, target := range targets {
		go func() {
			value, err := r.run(ctx, target)
			results <- result[T]{value: value, err: err}
		}()
	}

	values := make([]T, 0, len(targets))
	for range targets {
		result := <-results
		if result.err != nil {
			err = errors.Join(err, result.err)
			continue
		}

		values = append(values, result.value)
	}

	return values, err
}

type result[T any] struct {
	value T
	err   error
}
