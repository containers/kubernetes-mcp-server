package tempo

import (
	"context"
	"fmt"
	"net/http"
)

// Loader defines the interface for querying Tempo
type Loader interface {
	QueryV2(ctx context.Context, traceID string, opts QueryV2Options) (string, error)
	Search(ctx context.Context, opts SearchOptions) (string, error)
	SearchTagsV2(ctx context.Context, opts SearchTagsV2Options) (string, error)
	SearchTagValuesV2(ctx context.Context, tag string, opts SearchTagValuesV2Options) (string, error)
}

// RealLoader implements Loader using the Tempo HTTP API.
type RealLoader struct {
	client *TempoClient
}

var _ Loader = (*RealLoader)(nil)

const MaxTraceLimit = 1000
const MaxSpssLimit = 100
const MaxTagNamesLimit = 500
const MaxTagValuesLimit = 1000

func NewTempoLoader(httpClient *http.Client, url string) Loader {
	client := NewTempoClient(httpClient, url)

	return &RealLoader{
		client: client,
	}
}

func validateTimeRange(start, end int64) error {
	if start != 0 && end != 0 && start > end {
		return fmt.Errorf("invalid time range: start is after end")
	}
	return nil
}

func (r *RealLoader) QueryV2(ctx context.Context, traceID string, opts QueryV2Options) (string, error) {
	if err := validateTimeRange(opts.Start, opts.End); err != nil {
		return "", err
	}
	return r.client.QueryV2(ctx, traceID, opts)
}

func (r *RealLoader) Search(ctx context.Context, opts SearchOptions) (string, error) {
	if err := validateTimeRange(opts.Start, opts.End); err != nil {
		return "", err
	}
	if opts.Limit < 0 {
		return "", fmt.Errorf("invalid limit: must be non-negative")
	}
	if opts.Limit > MaxTraceLimit {
		return "", fmt.Errorf("requested search results limit %d is greater than max limit %d", opts.Limit, MaxTraceLimit)
	}
	if opts.Spss < 0 {
		return "", fmt.Errorf("invalid spss: must be non-negative")
	}
	if opts.Spss > MaxSpssLimit {
		return "", fmt.Errorf("requested spss limit %d is greater than max limit %d", opts.Spss, MaxSpssLimit)
	}
	return r.client.Search(ctx, opts)
}

func (r *RealLoader) SearchTagsV2(ctx context.Context, opts SearchTagsV2Options) (string, error) {
	if err := validateTimeRange(opts.Start, opts.End); err != nil {
		return "", err
	}
	if opts.Limit < 0 {
		return "", fmt.Errorf("invalid limit: must be non-negative")
	}
	if opts.Limit > MaxTagNamesLimit {
		return "", fmt.Errorf("requested tag names limit %d is greater than max limit %d", opts.Limit, MaxTagNamesLimit)
	}
	if opts.MaxStaleValues < 0 {
		return "", fmt.Errorf("invalid maxStaleValues: must be non-negative")
	}
	return r.client.SearchTagsV2(ctx, opts)
}

func (r *RealLoader) SearchTagValuesV2(ctx context.Context, tag string, opts SearchTagValuesV2Options) (string, error) {
	if err := validateTimeRange(opts.Start, opts.End); err != nil {
		return "", err
	}
	if opts.Limit < 0 {
		return "", fmt.Errorf("invalid limit: must be non-negative")
	}
	if opts.Limit > MaxTagValuesLimit {
		return "", fmt.Errorf("requested tag values limit %d is greater than max limit %d", opts.Limit, MaxTagValuesLimit)
	}
	if opts.MaxStaleValues < 0 {
		return "", fmt.Errorf("invalid maxStaleValues: must be non-negative")
	}
	return r.client.SearchTagValuesV2(ctx, tag, opts)
}
