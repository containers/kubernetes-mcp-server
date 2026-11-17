package metrics

import (
	"testing"

	ammodels "github.com/prometheus/alertmanager/api/v2/models"
)

func TestConvertAlert_PromotesRunbook(t *testing.T) {
	t.Parallel()
	got := convertAlert(&ammodels.GettableAlert{
		Alert: ammodels.Alert{
			Labels: ammodels.LabelSet{"alertname": "HighCPU"},
		},
		Annotations: ammodels.LabelSet{
			"runbook_url": "https://example.com/run",
			"summary":     "CPU high",
			"description": "Usage above threshold",
		},
	})
	if got.RunbookURL != "https://example.com/run" {
		t.Fatalf("RunbookURL = %q", got.RunbookURL)
	}
	if got.Summary != "CPU high" || got.Description != "Usage above threshold" {
		t.Fatalf("summary/description = %q / %q", got.Summary, got.Description)
	}
	if got.Annotations["runbook_url"] != "https://example.com/run" {
		t.Fatalf("annotations preserved: %+v", got.Annotations)
	}
}
