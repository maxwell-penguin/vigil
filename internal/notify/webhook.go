// Package notify posts SLO breach alerts to a Slack or Discord webhook.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"vigil/internal/models"
	"vigil/internal/selfmetrics"
)

const footerIconURL = "https://github.com/maxwell-penguin/vigil"

var httpClient = &http.Client{Timeout: 5 * time.Second}

type WebhookNotifier struct {
	URL  string
	Type string // "slack" or "discord"
}

func NewWebhookNotifier(url, webhookType string) *WebhookNotifier {
	return &WebhookNotifier{URL: url, Type: webhookType}
}

// Notify posts a breach summary to the configured webhook. Callers should
// log the returned error rather than treat it as fatal — a dead webhook
// must never stop the SLO checker from recording the alert.
func (w *WebhookNotifier) Notify(alert models.Alert, projectID string) (err error) {
	defer func() {
		if err != nil {
			selfmetrics.WebhookFailures.Add(1)
		}
	}()

	var payload any
	if w.Type == "discord" {
		payload = discordPayload(alert, projectID)
	} else {
		payload = slackPayload(alert, projectID)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

type slackMessage struct {
	Text        string            `json:"text"`
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color      string       `json:"color"`
	Fields     []slackField `json:"fields"`
	Footer     string       `json:"footer"`
	FooterIcon string       `json:"footer_icon"`
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

func slackPayload(a models.Alert, projectID string) slackMessage {
	return slackMessage{
		Text: fmt.Sprintf("🚨 *SLO Breach Detected* — %s", projectID),
		Attachments: []slackAttachment{{
			Color: "danger",
			Fields: []slackField{
				{Title: "SLO", Value: fmt.Sprintf("%.2f%% (target %.1f%%)", a.SLOPct, a.TargetPct), Short: true},
				{Title: "Budget Consumed", Value: fmt.Sprintf("%.1f%%", a.BudgetConsumedPct), Short: true},
				{Title: "Short Burn Rate", Value: fmt.Sprintf("%.1fx", a.ShortBurnRate), Short: true},
				{Title: "Long Burn Rate", Value: fmt.Sprintf("%.1fx", a.LongBurnRate), Short: true},
				{Title: "Detected At", Value: a.FiredAt.UTC().Format("2006-01-02 15:04:05 UTC"), Short: false},
			},
			Footer:     "Vigil SRE Toolkit",
			FooterIcon: footerIconURL,
		}},
	}
}

type discordMessage struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title     string         `json:"title"`
	Color     int            `json:"color"`
	Fields    []discordField `json:"fields"`
	Footer    discordFooter  `json:"footer"`
	Timestamp string         `json:"timestamp"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordFooter struct {
	Text string `json:"text"`
}

func discordPayload(a models.Alert, projectID string) discordMessage {
	return discordMessage{
		Embeds: []discordEmbed{{
			Title: fmt.Sprintf("🚨 SLO Breach Detected — %s", projectID),
			Color: 15158332, // Discord red
			Fields: []discordField{
				{Name: "SLO", Value: fmt.Sprintf("%.2f%% (target %.1f%%)", a.SLOPct, a.TargetPct), Inline: true},
				{Name: "Budget Consumed", Value: fmt.Sprintf("%.1f%%", a.BudgetConsumedPct), Inline: true},
				{Name: "Short Burn Rate", Value: fmt.Sprintf("%.1fx", a.ShortBurnRate), Inline: true},
				{Name: "Long Burn Rate", Value: fmt.Sprintf("%.1fx", a.LongBurnRate), Inline: true},
			},
			Footer:    discordFooter{Text: "Vigil SRE Toolkit"},
			Timestamp: a.FiredAt.UTC().Format(time.RFC3339),
		}},
	}
}
