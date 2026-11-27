package alertrunbook

import "testing"

func TestPromote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ann  map[string]string
		want Promoted
	}{
		{
			name: "runbook_url",
			ann: map[string]string{
				"runbook_url": "https://example.com/run",
				"summary":     "High CPU",
				"description": "CPU above threshold",
			},
			want: Promoted{
				RunbookURL:  "https://example.com/run",
				Summary:     "High CPU",
				Description: "CPU above threshold",
			},
		},
		{
			name: "empty",
			ann:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Promote(tt.ann)
			if got != tt.want {
				t.Fatalf("Promote() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
