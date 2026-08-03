package github

import (
	"fmt"
	"strings"
	"time"

	"vigil/internal/models"
	"vigil/internal/slo"
)

// Store is the subset of storage.Store the incident manager needs for dedup.
type Store interface {
	LatestAlert(projectID string) (models.Alert, bool, error)
}

// IncidentManager implements slo.Notifier by opening GitHub issues for breaches.
type IncidentManager struct {
	Client *Client
	Store  Store
}

func NewIncidentManager(client *Client, store Store) *IncidentManager {
	return &IncidentManager{Client: client, Store: store}
}

// NotifyBreach opens a new incident issue unless the previous one for this
// project is still open, per the dedup rule: check GitHub, not just our DB,
// since a maintainer may have closed the issue by hand.
func (m *IncidentManager) NotifyBreach(s models.SLO, status slo.Status, events []models.Event, now time.Time) (int, error) {
	last, ok, err := m.Store.LatestAlert(s.ProjectID)
	if err != nil {
		return 0, fmt.Errorf("latest alert: %w", err)
	}
	if ok && last.IssueNumber != 0 {
		issue, err := m.Client.GetIssue(last.IssueNumber)
		if err != nil {
			return 0, fmt.Errorf("get issue #%d: %w", last.IssueNumber, err)
		}
		if issue.State == "open" {
			return last.IssueNumber, nil // dedup: previous incident still active
		}
	}

	title := fmt.Sprintf("⚠️ SLO Breach Detected — %s", now.UTC().Format(time.RFC3339))
	body := postmortemBody(s, status, events, now)
	issue, err := m.Client.CreateIssue(title, body, []string{"incident", "slo-breach"})
	if err != nil {
		return 0, fmt.Errorf("create issue: %w", err)
	}
	return issue.Number, nil
}

func postmortemBody(s models.SLO, status slo.Status, events []models.Event, now time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Detection\n\n")
	fmt.Fprintf(&b, "- **Time of detection:** %s\n", now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- **SLO target:** %.3f%%\n", s.TargetPct)
	fmt.Fprintf(&b, "- **Current SLO:** %.3f%%\n", status.SLOPct)
	fmt.Fprintf(&b, "- **Error budget consumed:** %.2f%%\n", status.BudgetConsumedPct)
	fmt.Fprintf(&b, "- **Short window (5m) burn rate:** %.2f\n", status.ShortBurnRate)
	fmt.Fprintf(&b, "- **Long window (1h) burn rate:** %.2f\n\n", status.LongBurnRate)

	fmt.Fprintf(&b, "## Last %d Raw Events\n\n", len(events))
	if len(events) == 0 {
		b.WriteString("_No raw events available._\n\n")
	} else {
		b.WriteString("| Timestamp | Latency (ms) | Status | Error |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, e := range events {
			fmt.Fprintf(&b, "| %s | %d | %d | %t |\n",
				e.Timestamp.UTC().Format(time.RFC3339), e.LatencyMS, e.StatusCode, e.Error)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Impact\n\n_TBD_\n\n")
	b.WriteString("## Root Cause\n\n_TBD_\n\n")
	b.WriteString("## Timeline\n\n_TBD_\n\n")
	b.WriteString("## Action Items\n\n- [ ] \n")

	return b.String()
}
