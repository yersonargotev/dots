package provision

import "github.com/yersonargotev/dots/internal/state"

// StatusState summarizes the last known provisioning outcome for one selected
// Provisioner.
type StatusState string

const (
	StatusStatePending             StatusState = "pending"
	StatusStateProvisioned         StatusState = "provisioned"
	StatusStateFailed              StatusState = "failed"
	StatusStateMissingDependencies StatusState = "missing-dependencies"
)

// SummaryState summarizes the selected Provisioners for a profile.
type SummaryState string

const (
	SummaryStateNone      SummaryState = "none"
	SummaryStatePending   SummaryState = "pending"
	SummaryStateCompleted SummaryState = "completed"
	SummaryStateFailed    SummaryState = "failed"
)

// StatusSummary is the profile-level Provisioner completion state surfaced by
// diagnostics.
type StatusSummary struct {
	State     SummaryState `json:"state"`
	Completed int          `json:"completed"`
	Pending   int          `json:"pending"`
	Failed    int          `json:"failed"`
}

// StatusItem combines the resolved Provisioner command with its last persisted
// run result, if any.
type StatusItem struct {
	Tool       string      `json:"tool"`
	Executable string      `json:"executable"`
	Args       []string    `json:"args"`
	Targets    []string    `json:"targets,omitempty"`
	Status     StatusState `json:"status"`
	LastRunAt  string      `json:"last_run_at,omitempty"`
	Missing    []string    `json:"missing,omitempty"`
}

// StatusReport is the read-only diagnostic view of Provisioner completion.
type StatusReport struct {
	Profile string        `json:"profile"`
	Summary StatusSummary `json:"summary"`
	Items   []StatusItem  `json:"items"`
}

// HasFindings reports whether Provisioners are pending or failed for the active
// profile. A profile with no selected Provisioners is not a finding.
func (r StatusReport) HasFindings() bool {
	return r.Summary.State == SummaryStatePending || r.Summary.State == SummaryStateFailed
}

// BuildStatus joins the selected Provisioner plan with persisted Installation
// Metadata to expose whether Provisioners are pending, failed, or completed.
func BuildStatus(p Plan, meta state.Metadata) StatusReport {
	report := StatusReport{Profile: p.Profile}
	if len(p.Steps) == 0 {
		report.Summary.State = SummaryStateNone
		return report
	}

	for _, step := range p.Steps {
		item := StatusItem{
			Tool:       step.Tool,
			Executable: step.Executable,
			Args:       append([]string(nil), step.Args...),
			Targets:    append([]string(nil), step.Targets...),
			Status:     StatusStatePending,
		}
		if rec, ok := meta.FindProvisioner(p.Profile, step.Tool, step.Executable, step.Args); ok {
			item.Status = StatusState(rec.Status)
			item.LastRunAt = rec.LastRunAt
			item.Missing = append([]string(nil), rec.Missing...)
		}
		report.Items = append(report.Items, item)
		switch item.Status {
		case StatusStateProvisioned:
			report.Summary.Completed++
		case StatusStateFailed, StatusStateMissingDependencies:
			report.Summary.Failed++
		default:
			report.Summary.Pending++
		}
	}

	switch {
	case report.Summary.Failed > 0:
		report.Summary.State = SummaryStateFailed
	case report.Summary.Pending > 0:
		report.Summary.State = SummaryStatePending
	default:
		report.Summary.State = SummaryStateCompleted
	}
	return report
}
