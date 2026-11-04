package metrics

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/go-openapi/strfmt"
	ammodels "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/common/model"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/metrics/prometheus"
)

const (
	defaultSilenceDuration  = 2 * time.Hour
	defaultSilenceCreatedBy = "obs-mcp"
)

type silenceWriteInput struct {
	ID        string
	Comment   string
	CreatedBy string
	StartsAt  time.Time
	EndsAt    time.Time
	Matchers  ammodels.Matchers
}

func createSilenceHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	in, err := parseSilenceWrite(params.Request.GetArguments(), nil)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return postSilence(params, in)
}

func updateSilenceHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	id, err := parseSilenceID(params.Request.GetArguments())
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}
	existing, err := amClient.GetSilence(params.Context, id)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if existing == nil {
		return api.NewToolCallResult("", fmt.Errorf("silence %s not found", id)), nil
	}

	in, err := parseSilenceWrite(params.Request.GetArguments(), existing)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	in.ID = id
	return postSilence(params, in)
}

func deleteSilenceHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	id, err := parseSilenceID(params.Request.GetArguments())
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}
	if err := amClient.DeleteSilence(params.Context, id); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(SilenceWriteOutput{SilenceID: id}, nil), nil
}

func parseSilenceID(args map[string]any) (string, error) {
	id := strings.TrimSpace(stringArg(args, "silence_id"))
	if id == "" {
		return "", fmt.Errorf("silence_id is required")
	}
	if !strfmt.IsUUID(id) {
		return "", fmt.Errorf("silence_id must be a UUID")
	}
	return id, nil
}

func postSilence(params api.ToolHandlerParams, in *silenceWriteInput) (*api.ToolCallResult, error) {
	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	startsAt := strfmt.DateTime(in.StartsAt)
	endsAt := strfmt.DateTime(in.EndsAt)
	createdBy := in.CreatedBy
	comment := in.Comment
	body := &ammodels.PostableSilence{
		ID: in.ID,
		Silence: ammodels.Silence{
			Matchers:  in.Matchers,
			StartsAt:  &startsAt,
			EndsAt:    &endsAt,
			CreatedBy: &createdBy,
			Comment:   &comment,
		},
	}

	klogutil.FromContext(params.Context).Info("postSilence called", "silence_id", in.ID, "matcher_count", len(in.Matchers))
	silenceID, err := amClient.PostSilence(params.Context, body)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(SilenceWriteOutput{SilenceID: silenceID}, nil), nil
}

func parseSilenceWrite(args map[string]any, existing *ammodels.GettableSilence) (*silenceWriteInput, error) {
	if args == nil {
		args = map[string]any{}
	}

	comment := strings.TrimSpace(stringArg(args, "comment"))
	createdBy := strings.TrimSpace(stringArg(args, "createdBy"))
	startsAt, err := optionalTimestamp(args, "startsAt")
	if err != nil {
		return nil, err
	}
	endsAt, err := optionalTimestamp(args, "endsAt")
	if err != nil {
		return nil, err
	}
	duration, err := optionalDuration(args, "duration")
	if err != nil {
		return nil, err
	}
	if !endsAt.IsZero() && duration != 0 {
		return nil, fmt.Errorf("endsAt and duration cannot both be set")
	}

	matchers, err := parseSilenceMatchers(args)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if comment == "" {
			comment = strings.TrimSpace(ptr.Deref(existing.Comment, ""))
		}
		if createdBy == "" {
			createdBy = strings.TrimSpace(ptr.Deref(existing.CreatedBy, ""))
		}
		if startsAt.IsZero() && existing.StartsAt != nil {
			startsAt = time.Time(*existing.StartsAt)
		}
		if endsAt.IsZero() && duration == 0 && existing.EndsAt != nil {
			endsAt = time.Time(*existing.EndsAt)
		}
		if len(matchers) == 0 {
			matchers = existing.Matchers
		}
	}

	if comment == "" {
		return nil, fmt.Errorf("comment is required")
	}
	if createdBy == "" {
		createdBy = defaultSilenceCreatedBy
	}
	if startsAt.IsZero() {
		startsAt = time.Now()
	}
	if duration != 0 {
		endsAt = startsAt.Add(duration)
	}
	if endsAt.IsZero() {
		endsAt = startsAt.Add(defaultSilenceDuration)
	}
	if !endsAt.After(startsAt) {
		return nil, fmt.Errorf("endsAt must be after startsAt")
	}
	if len(matchers) == 0 {
		return nil, fmt.Errorf("at least one matcher is required (matchers or labels)")
	}

	return &silenceWriteInput{
		Comment:   comment,
		CreatedBy: createdBy,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		Matchers:  matchers,
	}, nil
}

func parseSilenceMatchers(args map[string]any) (ammodels.Matchers, error) {
	labels, err := stringMapArg(args, "labels")
	if err != nil {
		return nil, err
	}
	keys := slices.Collect(maps.Keys(labels))
	slices.Sort(keys)

	out := make(ammodels.Matchers, 0, len(keys))
	for _, key := range keys {
		value := labels[key]
		if key == "" || value == "" {
			return nil, fmt.Errorf("labels keys and values must be non-empty")
		}
		out = append(out, equalityMatcher(key, value))
	}

	raw, ok := args["matchers"]
	if !ok || raw == nil {
		return out, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("matchers must be an array")
	}
	for i, item := range items {
		matcher, err := parseSilenceMatcher(item)
		if err != nil {
			return nil, fmt.Errorf("matchers[%d]: %w", i, err)
		}
		out = append(out, matcher)
	}
	return out, nil
}

func parseSilenceMatcher(raw any) (*ammodels.Matcher, error) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object with name and value")
	}
	name := strings.TrimSpace(stringArg(fields, "name"))
	value := stringArg(fields, "value")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	isRegex, err := boolArgDefault(fields, "isRegex", false)
	if err != nil {
		return nil, err
	}
	isEqual, err := boolArgDefault(fields, "isEqual", true)
	if err != nil {
		return nil, err
	}
	return &ammodels.Matcher{
		Name:    &name,
		Value:   &value,
		IsRegex: &isRegex,
		IsEqual: &isEqual,
	}, nil
}

func equalityMatcher(name, value string) *ammodels.Matcher {
	isRegex := false
	isEqual := true
	return &ammodels.Matcher{
		Name:    &name,
		Value:   &value,
		IsRegex: &isRegex,
		IsEqual: &isEqual,
	}
}

func optionalTimestamp(args map[string]any, key string) (time.Time, error) {
	raw := strings.TrimSpace(stringArg(args, key))
	if raw == "" {
		return time.Time{}, nil
	}
	ts, err := prometheus.ParseTimestamp(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s: %w", key, err)
	}
	return ts, nil
}

func optionalDuration(args map[string]any, key string) (time.Duration, error) {
	raw := strings.TrimSpace(stringArg(args, key))
	if raw == "" {
		return 0, nil
	}
	parsed, err := model.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0", key)
	}
	return time.Duration(parsed), nil
}

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return s
}

func stringMapArg(args map[string]any, key string) (map[string]string, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object of string values", key)
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be a string", key, k)
		}
		out[k] = s
	}
	return out, nil
}

func boolArgDefault(args map[string]any, key string, fallback bool) (bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return b, nil
}
