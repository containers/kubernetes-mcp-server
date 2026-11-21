package alertrunbook

// Promoted holds well-known alert annotation fields for triage.
type Promoted struct {
	RunbookURL  string
	Summary     string
	Description string
}

const runbookURLAnnotation = "runbook_url"

// Promote copies runbook_url, summary, and description from alert annotations.
func Promote(annotations map[string]string) Promoted {
	var out Promoted
	if len(annotations) == 0 {
		return out
	}
	out.RunbookURL = annotations[runbookURLAnnotation]
	out.Summary = annotations["summary"]
	out.Description = annotations["description"]
	return out
}
