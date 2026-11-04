package metrics

import (
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	ammodels "github.com/prometheus/alertmanager/api/v2/models"
	"k8s.io/utils/ptr"
)

func TestParseSilenceWrite_CreateDefaults(t *testing.T) {
	in, err := parseSilenceWrite(map[string]any{
		"comment": "maintenance",
		"labels": map[string]any{
			"alertname": "Watchdog",
			"namespace": "app",
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Comment != "maintenance" {
		t.Errorf("comment = %q", in.Comment)
	}
	if in.CreatedBy != defaultSilenceCreatedBy {
		t.Errorf("createdBy = %q", in.CreatedBy)
	}
	if got := in.EndsAt.Sub(in.StartsAt); got != defaultSilenceDuration {
		t.Errorf("duration = %v, want %v", got, defaultSilenceDuration)
	}
	if len(in.Matchers) != 2 {
		t.Fatalf("matchers = %d, want 2", len(in.Matchers))
	}
	if ptr.Deref(in.Matchers[0].Name, "") != "alertname" || ptr.Deref(in.Matchers[0].Value, "") != "Watchdog" {
		t.Errorf("first matcher = %+v", in.Matchers[0])
	}
	if ptr.Deref(in.Matchers[1].Name, "") != "namespace" {
		t.Errorf("second matcher name = %q", ptr.Deref(in.Matchers[1].Name, ""))
	}
}

func TestParseSilenceWrite_Duration(t *testing.T) {
	in, err := parseSilenceWrite(map[string]any{
		"comment":   "window",
		"createdBy": "alice",
		"duration":  "30m",
		"matchers": []any{
			map[string]any{"name": "alertname", "value": "HighCPU"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.CreatedBy != "alice" {
		t.Errorf("createdBy = %q", in.CreatedBy)
	}
	if got := in.EndsAt.Sub(in.StartsAt); got != 30*time.Minute {
		t.Errorf("duration = %v, want 30m", got)
	}
}

func TestParseSilenceWrite_RejectsEndsAtAndDuration(t *testing.T) {
	_, err := parseSilenceWrite(map[string]any{
		"comment":  "x",
		"endsAt":   "NOW+1h",
		"duration": "2h",
		"labels":   map[string]any{"alertname": "A"},
	}, nil)
	if err == nil || err.Error() != "endsAt and duration cannot both be set" {
		t.Fatalf("got %v", err)
	}
}

func TestParseSilenceWrite_RequiresCommentAndMatchers(t *testing.T) {
	_, err := parseSilenceWrite(map[string]any{
		"labels": map[string]any{"alertname": "A"},
	}, nil)
	if err == nil || err.Error() != "comment is required" {
		t.Fatalf("got %v", err)
	}

	_, err = parseSilenceWrite(map[string]any{"comment": "x"}, nil)
	if err == nil || err.Error() != "at least one matcher is required (matchers or labels)" {
		t.Fatalf("got %v", err)
	}
}

func TestParseSilenceWrite_UpdateKeepsExisting(t *testing.T) {
	starts := strfmt.DateTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	ends := strfmt.DateTime(time.Time(starts).Add(time.Hour))
	existing := &ammodels.GettableSilence{
		Silence: ammodels.Silence{
			Comment:   new("old"),
			CreatedBy: new("bob"),
			Matchers: ammodels.Matchers{
				equalityMatcher("alertname", "Old"),
			},
			StartsAt: &starts,
			EndsAt:   &ends,
		},
	}

	in, err := parseSilenceWrite(map[string]any{
		"silence_id": "11111111-1111-1111-1111-111111111111",
		"duration":   "4h",
	}, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Comment != "old" || in.CreatedBy != "bob" {
		t.Errorf("comment/createdBy = %q / %q", in.Comment, in.CreatedBy)
	}
	if ptr.Deref(in.Matchers[0].Value, "") != "Old" {
		t.Errorf("matcher value = %q", ptr.Deref(in.Matchers[0].Value, ""))
	}
	if got := in.EndsAt.Sub(in.StartsAt); got != 4*time.Hour {
		t.Errorf("duration = %v, want 4h", got)
	}
}

func TestParseSilenceMatcher_Defaults(t *testing.T) {
	m, err := parseSilenceMatcher(map[string]any{
		"name":  "severity",
		"value": "critical",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ptr.Deref(m.IsRegex, true) {
		t.Error("expected isRegex false")
	}
	if !ptr.Deref(m.IsEqual, false) {
		t.Error("expected isEqual true")
	}
}
