package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vigil/internal/models"
)

func testAlert() models.Alert {
	return models.Alert{
		ProjectID:         "demo",
		FiredAt:           time.Date(2026, 8, 10, 2, 20, 0, 0, time.UTC),
		BudgetConsumedPct: 62.4,
		SLOPct:            85.2,
		TargetPct:         90.0,
		ShortBurnRate:     18.5,
		LongBurnRate:      3.2,
	}
}

func TestNotifySlackPayload(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "slack")
	if err := n.Notify(testAlert(), "demo"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	var msg slackMessage
	if err := json.Unmarshal(gotBody, &msg); err != nil {
		t.Fatalf("unmarshal slack payload: %v\nbody: %s", err, gotBody)
	}

	if msg.Text != "🚨 *SLO Breach Detected* — demo" {
		t.Errorf("text = %q", msg.Text)
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(msg.Attachments))
	}
	att := msg.Attachments[0]
	if att.Color != "danger" {
		t.Errorf("color = %q, want danger", att.Color)
	}
	if att.Footer != "Vigil SRE Toolkit" {
		t.Errorf("footer = %q", att.Footer)
	}
	if len(att.Fields) != 5 {
		t.Fatalf("fields = %d, want 5", len(att.Fields))
	}
	wantFields := map[string]string{
		"SLO":             "85.20% (target 90.0%)",
		"Budget Consumed": "62.4%",
		"Short Burn Rate": "18.5x",
		"Long Burn Rate":  "3.2x",
		"Detected At":     "2026-08-10 02:20:00 UTC",
	}
	for _, f := range att.Fields {
		want, ok := wantFields[f.Title]
		if !ok {
			t.Errorf("unexpected field %q", f.Title)
			continue
		}
		if f.Value != want {
			t.Errorf("field %q = %q, want %q", f.Title, f.Value, want)
		}
	}
	// Detected At is the only non-short field; the rest are short: true.
	for _, f := range att.Fields {
		wantShort := f.Title != "Detected At"
		if f.Short != wantShort {
			t.Errorf("field %q short = %v, want %v", f.Title, f.Short, wantShort)
		}
	}
}

func TestNotifyDiscordPayload(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "discord")
	if err := n.Notify(testAlert(), "demo"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	var msg discordMessage
	if err := json.Unmarshal(gotBody, &msg); err != nil {
		t.Fatalf("unmarshal discord payload: %v\nbody: %s", err, gotBody)
	}

	if len(msg.Embeds) != 1 {
		t.Fatalf("embeds = %d, want 1", len(msg.Embeds))
	}
	embed := msg.Embeds[0]
	if embed.Title != "🚨 SLO Breach Detected — demo" {
		t.Errorf("title = %q", embed.Title)
	}
	if embed.Color != 15158332 {
		t.Errorf("color = %d, want 15158332", embed.Color)
	}
	if embed.Footer.Text != "Vigil SRE Toolkit" {
		t.Errorf("footer text = %q", embed.Footer.Text)
	}
	if embed.Timestamp != "2026-08-10T02:20:00Z" {
		t.Errorf("timestamp = %q, want RFC3339", embed.Timestamp)
	}
	if len(embed.Fields) != 4 {
		t.Fatalf("fields = %d, want 4", len(embed.Fields))
	}
	wantFields := map[string]string{
		"SLO":             "85.20% (target 90.0%)",
		"Budget Consumed": "62.4%",
		"Short Burn Rate": "18.5x",
		"Long Burn Rate":  "3.2x",
	}
	for _, f := range embed.Fields {
		want, ok := wantFields[f.Name]
		if !ok {
			t.Errorf("unexpected field %q", f.Name)
			continue
		}
		if f.Value != want {
			t.Errorf("field %q = %q, want %q", f.Name, f.Value, want)
		}
		if !f.Inline {
			t.Errorf("field %q inline = false, want true", f.Name)
		}
	}
}

func TestNotifyErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "slack")
	if err := n.Notify(testAlert(), "demo"); err == nil {
		t.Fatal("Notify: expected error on 500 response, got nil")
	}
}
